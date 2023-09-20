// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package manager

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/services/cros/cellular"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

// Timeout for methods of Tast fixture.
const (
	setUpTimeout    = 6 * time.Minute
	tearDownTimeout = 3 * time.Minute
	resetTimeout    = 1 * time.Second
	postTestTimeout = 1 * time.Minute
	preTestTimeout  = 1 * time.Minute
	testURLIPv4     = "ipv6-test.com"
	// Use callbox DAU address since IPv6 is not supported in lab (b/255775799).
	testURLIPv6 = DAUAddressIPv6
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:     "callboxManagedFixture",
		Desc:     "Cellular fixture with a Callbox managed by a Callbox Manager",
		Contacts: []string{
			// None yet, fixture is still preliminary
		},
		Impl:            &TestFixture{},
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		PostTestTimeout: postTestTimeout,
		PreTestTimeout:  preTestTimeout,
		TearDownTimeout: tearDownTimeout,
		ServiceDeps:     []string{"tast.cros.cellular.RemoteCellularService"},
		Vars:            []string{"callboxManager", "callbox", "companionCount"},
	})
}

// TestFixture is the test fixture used for callboxManagedFixture fixtures.
type TestFixture struct {
	fcm                  *forwardedCallboxManager
	rpcClient            *rpc.Client
	CallboxManagerClient *CallboxManagerClient
	RemoteCellularClient cellular.RemoteCellularServiceClient
	InterfaceName        string
	Vars                 fixtureVars
}

type fixtureVars struct {
	CallboxManager string
	Callbox        string
}

// SetUp sets up the test fixture and connects to the CallboxManager.
func (tf *TestFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	dut := s.DUT()

	// Parse Vars
	callbox, ok := s.Var("callbox")
	if !ok || callbox == "" {
		testing.ContextLog(ctx, "No callbox specified, deducing from DUT name")
		var err error
		if callbox, err = callboxHostName(dut); err != nil {
			s.Fatal("Failed to determine callbox hostname: ", err)
		}
	}
	tf.Vars.Callbox = callbox
	testing.ContextLogf(ctx, "Using callbox: %q", tf.Vars.Callbox)

	callboxManager, ok := s.Var("callboxManager")
	if !ok || callboxManager == "" {
		testing.ContextLog(ctx, "No callbox specified, using default")
		callboxManager = labProxyHostname
	}
	tf.Vars.CallboxManager = callboxManager
	testing.ContextLogf(ctx, "Using callboxManager: %q", tf.Vars.CallboxManager)

	// Initialize CallboxManagerClient
	if tf.Vars.CallboxManager == labProxyHostname {
		// Tunnel to Callbox Manager on labProxyHostname
		var err error
		tf.fcm, err = newForwardToLabCallboxManager(ctx, dut.KeyDir(), dut.KeyFile())
		if err != nil {
			s.Fatalf("Failed to open tunnel to Callbox Manager on %q, err: %v", labProxyHostname, err)
		}
		tf.CallboxManagerClient = &CallboxManagerClient{
			baseURL:        "http://" + tf.fcm.LocalAddress(),
			defaultCallbox: tf.Vars.Callbox,
		}
	} else {
		// Callbox Manager directly accessible
		tf.CallboxManagerClient = &CallboxManagerClient{
			baseURL:        "http://" + tf.Vars.CallboxManager,
			defaultCallbox: tf.Vars.Callbox,
		}
	}

	if err := disableCompanionDUTs(ctx, s); err != nil {
		s.Fatal("Failed to disable companion DUTs: ", err)
	}

	if err := tf.initRemoteClient(ctx, s.DUT(), s.RPCHint()); err != nil {
		s.Fatal("Failed to initialize remote cellular client: ", err)
	}

	if resp, err := tf.RemoteCellularClient.QueryInterface(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to query cellular interface: ", err)
	} else {
		s.Logf("Using cellular interface %q", resp.Name)
		tf.InterfaceName = resp.Name
	}

	return tf
}

// initRemoteClient initializes/reinitializes the remote cellular client which may be disconnected due to a network error.
func (tf *TestFixture) initRemoteClient(ctx context.Context, dut *dut.DUT, hint *testing.RPCHint) error {
	// Close previous client if it already exists.
	if tf.RemoteCellularClient != nil {
		if _, err := tf.RemoteCellularClient.TearDown(ctx, &empty.Empty{}); err != nil {
			testing.ContextLog(ctx, "Failed to tear down cellular remote service: ", err)
		}

		if err := tf.rpcClient.Close(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to close DUT RPC client: ", err)
		}
		tf.RemoteCellularClient = nil
		tf.rpcClient = nil
	}

	cl, err := rpc.Dial(ctx, dut, hint)
	if err != nil {
		return errors.Wrap(err, "failed to connect to the RPC service on the DUT")
	}
	tf.rpcClient = cl

	tf.RemoteCellularClient = cellular.NewRemoteCellularServiceClient(cl.Conn)
	if _, err := tf.RemoteCellularClient.SetUp(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to initialize cellular shill service on DUT")
	}
	return nil
}

// ConnectToCallbox function handles initial test setup and wraps parameters.
func (tf *TestFixture) ConnectToCallbox(ctx context.Context, dutConn *ssh.Conn, configureRequestBody *ConfigureCallboxRequestBody) error {
	// Disable and then re-enable cellular on DUT.
	if _, err := tf.RemoteCellularClient.Disable(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to disable DUT cellular")
	}

	// Preform callbox simulation.
	if err := tf.CallboxManagerClient.ConfigureCallbox(ctx, configureRequestBody); err != nil {
		return errors.Wrap(err, "failed to configure callbox")
	}

	// Allow for cellular simulation to start before turning on mobile data.
	errCh := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := tf.CallboxManagerClient.BeginSimulation(ctx, nil); err != nil {
			errCh <- errors.Wrap(err, "failed to begin callbox simulation")
		}
	}()
	// GoBigSleepLint (b/229419538): Wait for callbox simulation to start before turning on cellular.
	testing.Sleep(ctx, time.Second*10)
	if _, err := tf.RemoteCellularClient.Enable(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to enable DUT cellular")
	}

	wg.Wait()
	close(errCh)
	if len(errCh) > 0 {
		return <-errCh
	}

	// now attached but not connected, toggle modem and connect to make sure
	// everything is synced properly between the callbox and DUT
	if err := tf.ToggleConnection(ctx); err != nil {
		return errors.Wrap(err, "failed to toggle cellular connection")
	}

	// Now that we're connected, update the interface name.
	resp, err := tf.RemoteCellularClient.QueryInterface(ctx, &empty.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to query cellular interface")
	}

	testing.ContextLogf(ctx, "Using cellular interface %q", resp.Name)
	tf.InterfaceName = resp.Name

	// verify cellular connection by curling a website
	retryCount := 0
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		retryCount++
		testing.ContextLogf(ctx, "verifying IP connectivity, attempt: %d", retryCount)
		_, _, err := tf.VerifyConnectivity(ctx)
		return err
	}, &testing.PollOptions{Timeout: time.Minute, Interval: time.Second}); err != nil {
		return errors.Wrap(err, "failed to verify cellular connectivity")
	}
	return nil
}

// VerifyConnectivity verifies the DUT has network connectivity.
func (tf *TestFixture) VerifyConnectivity(ctx context.Context) (ipv4, ipv6 bool, err error) {
	req := cellular.VerifyIPConnectivityRequest{
		IPv4Address: testURLIPv4,
		IPv6Address: testURLIPv6,
	}

	resp, err := tf.RemoteCellularClient.VerifyIPConnectivity(ctx, &req)
	if err != nil {
		return false, false, errors.Wrap(err, "failed to verify network connectivity")
	} else if !resp.Ipv4 && !resp.Ipv6 {
		return false, false, errors.New("no IPv4 or IPv6 connectivity found")
	}
	return resp.Ipv4, resp.Ipv6, nil
}

// ToggleConnection disables and then re-enables the device, and then reconnects to the default cellular service.
func (tf *TestFixture) ToggleConnection(ctx context.Context) error {
	if _, err := tf.RemoteCellularClient.Disable(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to disable cellular service")
	}
	if _, err := tf.RemoteCellularClient.Enable(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to enable cellular service")
	}
	if _, err := tf.RemoteCellularClient.Connect(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to connect to cellular service")
	}
	return nil
}

// callboxHostName derives the hostname of the callbox from the dut's hostname.
//
// Callbox DUT hostnames follow the convention: <callbox_hostname>-host<host_number>
// e.g. a callbox with the name "chromeos1-donutlab-callbox1" may support the following DUTs:
// "chromeos1-donutlab-callbox1-host1", "chromeos1-donutlab-callbox1-host2", ...
func callboxHostName(dut *dut.DUT) (string, error) {
	dutHost := dut.HostName()
	if host, _, err := net.SplitHostPort(dutHost); err == nil {
		dutHost = host
	}

	dutHost = strings.TrimSuffix(dutHost, ".cros")
	if dutHost == "localhost" {
		return "", errors.Errorf("unable to parse hostname from: %q, localhost not supported", dutHost)
	}

	if ip := net.ParseIP(dutHost); ip != nil {
		return "", errors.Errorf("unable to parse hostname from: %q, ip:port format not supported", dutHost)
	}

	hostname := strings.Split(dutHost, "-")
	if len(hostname) < 2 {
		return "", errors.Errorf("unable to parse hostname from: %q, unknown name format", dutHost)
	}

	// CallboxManager expects callbox hostnames to end in .cros
	hostname = hostname[0 : len(hostname)-1]
	return fmt.Sprintf("%s.cros", strings.Join(hostname, "-")), nil
}

// Reset does nothing currently, but is required for the test fixture.
func (tf *TestFixture) Reset(ctx context.Context) error {
	return nil
}

// PreTest initializes the test fixture before each test run.
func (tf *TestFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	if tf.RemoteCellularClient == nil {
		// Remote client not running, may have failed a previous initialization.
		if err := tf.initRemoteClient(ctx, s.DUT(), s.RPCHint()); err != nil {
			s.Fatal("Failed to initialize remote cellular client: ", err)
		}
		if _, err := tf.RemoteCellularClient.PreTest(ctx, &empty.Empty{}); err != nil {
			s.Fatal("Failed PreTest initialization on remote client: ", err)
		}
	} else if _, err := tf.RemoteCellularClient.PreTest(ctx, &empty.Empty{}); err != nil {
		// Failed to call pre-test, remote client may have been disconnected so attempt to restart it.
		if err := tf.initRemoteClient(ctx, s.DUT(), s.RPCHint()); err != nil {
			s.Fatal("Failed to initialize remote cellular client: ", err)
		}

		// Attempt pre-test a second time.
		if _, err := tf.RemoteCellularClient.PreTest(ctx, &empty.Empty{}); err != nil {
			s.Fatal("Failed PreTest initialization on remote client: ", err)
		}
	}
}

// PostTest cleans up the test fixture after each test run.
func (tf TestFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	if _, err := tf.RemoteCellularClient.PostTest(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed PostTest cleanup on remote client: ", err)
	}
}

// TearDown releases resources held open by the test fixture.
func (tf *TestFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if tf.fcm != nil {
		if err := tf.fcm.Close(ctx); err != nil {
			s.Error("Failed to close tunnel to CallboxManager: ", err)
		}
	}

	if _, err := tf.RemoteCellularClient.TearDown(ctx, &empty.Empty{}); err != nil {
		s.Error("Failed to tear down cellular remote service: ", err)
	}

	if err := tf.rpcClient.Close(ctx); err != nil {
		s.Error("Failed to close DUT RPC client: ", err)
	}
}

// disableCompanionDUTs disables all "companion" DUTs that are connected to the same callbox.
func disableCompanionDUTs(ctx context.Context, s *testing.FixtState) error {
	str, ok := s.Var("companionCount")
	if !ok {
		return nil
	}

	companionCount, err := strconv.Atoi(str)
	if err != nil {
		return errors.Wrapf(err, "failed to parse integer variable: %q", str)
	}

	for i := 0; i < companionCount; i++ {
		companionDUT := s.CompanionDUT(fmt.Sprintf("cd%d", i+1))
		if companionDUT == nil {
			return errors.Errorf("failed to get companion DUT cd%d", i+1)
		}

		s.Log("Connecting to companion DUT: ", companionDUT.HostName())
		companionCl, err := rpc.Dial(ctx, companionDUT, s.RPCHint())
		if err != nil {
			s.Fatal("Failed to connect to the RPC service on the companion DUT: ", err)
		}
		defer companionCl.Close(ctx)

		// Create a new remote cellular client on companion DUT and disable it.
		service := cellular.NewRemoteCellularServiceClient(companionCl.Conn)
		if _, err := service.SetUp(ctx, &empty.Empty{}); err != nil {
			return errors.Wrapf(err, "failed to initialize remote cellular client on companion DUT: %d", i+1)
		}

		s.Log("Disabling cellular on companion DUT: ", companionDUT.HostName())
		if _, err := service.Disable(ctx, &empty.Empty{}); err != nil {
			return errors.Wrapf(err, "failed to disable companion DUT: %d", i+1)
		}
	}

	return nil
}
