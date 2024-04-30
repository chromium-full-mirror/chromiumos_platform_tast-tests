// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package uwb

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/android"
	"go.chromium.org/tast-tests/cros/common/uwb"
	us "go.chromium.org/tast-tests/cros/services/cros/uwb"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	resetTimeout = 60 * time.Second
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            "uwbMixedPeerRemote",
		BugComponent:    "b:1135853",
		Desc:            "Fixture for setting up mixed android/CrOS device testbeds",
		Contacts:        []string{"chromeos-uwb-team@google.com"},
		Impl:            &mixedPeerRemoteFixture{},
		ServiceDeps:     []string{"tast.cros.uwb.UwbService"},
		SetUpTimeout:    3 * time.Minute,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: resetTimeout,
		Vars:            []string{"uwb.mainHost", "uwb.crosHosts"},
	})
}

// FixtData holds information made available to tests that specify this Fixture.
type FixtData struct {
	PhoneIPs    []string
	CrosClients []*us.UwbServiceClient
	CrosHosts   []string
	MainHost    string
}

// phonePeer represents a phone peer connection.
type phonePeer struct {
	labToDrone *ssh.Forwarder
	droneToDUT *ssh.Forwarder
}

// crosDUT holds a dut instance and its corresponding UwbServiceClient
type crosDUT struct {
	dut    *dut.DUT
	client *us.UwbServiceClient
}

type mixedPeerRemoteFixture struct {
	labstation *ssh.Conn
	phonePeers []*phonePeer

	crosDUTs []*crosDUT
}

func (f *mixedPeerRemoteFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	fixtData := &FixtData{}

	//Set mainHost in FixtData
	if mainHostName, ok := s.Var("uwb.mainHost"); ok {
		fixtData.MainHost = mainHostName
	} else {
		dutHost := s.DUT().HostName()
		if host, _, err := net.SplitHostPort(dutHost); err == nil {
			dutHost = host
		}
		fixtData.MainHost = dutHost
	}

	//Set crosHosts in FixtData
	if crosHostsNames, ok := s.Var("uwb.crosHosts"); ok {
		fixtData.CrosHosts = strings.Split(crosHostsNames, ",")
	} else {
		// No crosHosts var passed in, just use host names directly from DUTs.
		fixtData.CrosHosts = make([]string, 0, len(s.CompanionDUTs()))
		for _, dut := range s.CompanionDUTs() {
			dutHost := dut.HostName()
			if host, _, err := net.SplitHostPort(dutHost); err == nil {
				dutHost = host
			}
			fixtData.CrosHosts = append(fixtData.CrosHosts, dutHost)
		}
	}

	//main Cros DUT setup
	svc, err := remoteDUTSetUp(ctx, s, s.DUT())
	if err != nil {
		s.Fatal("Main DUT set up error: ", err)
	}

	fixtData.CrosClients = []*us.UwbServiceClient{svc}
	f.crosDUTs = []*crosDUT{&crosDUT{
		dut:    s.DUT(),
		client: svc,
	}}

	//other Cros DUT setups
	for role, dut := range s.CompanionDUTs() {
		svc, err := remoteDUTSetUp(ctx, s, dut)
		if err != nil {
			s.Fatal(role+" set up error: ", err)
		}
		fixtData.CrosClients = append(fixtData.CrosClients, svc)

		f.crosDUTs = append(f.crosDUTs, &crosDUT{
			dut:    dut,
			client: svc,
		})
	}
	s.Logf("CrOS device count: %d", len(f.crosDUTs))

	// Get Android companion DUT info
	if companions, err := android.Companions(); err == nil {
		if err := f.setupLabstation(ctx, s, companions[0].AssociatedHostname); err != nil {
			s.Fatal("Failed to setup adb on labstation")
			return nil
		}

		if err := f.setupADBPeers(ctx, s, companions); err != nil {
			s.Fatal("Failed to setup adb peers: ", err)
			return nil
		}
	}

	fixtData.PhoneIPs = make([]string, len(f.phonePeers))

	s.Logf("Android device count: %d", len(f.phonePeers))
	for i := 0; i < len(f.phonePeers); i++ {
		fixtData.PhoneIPs[i] = f.phonePeers[i].droneToDUT.ListenAddr().String()
	}

	return fixtData
}

func (f *mixedPeerRemoteFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	for _, phone := range f.phonePeers {
		if err := phone.labToDrone.Close(); err != nil {
			s.Error("Failed to close forwarded labstation connection: ", err)
		}
		if err := phone.droneToDUT.Close(); err != nil {
			s.Error("Failed to close forwarded drone connection: ", err)
		}
	}

	if f.labstation != nil {
		if err := f.labstation.Close(ctx); err != nil {
			s.Error("Failed to close labstation connection: ", err)
		}
	}

	for i := 0; i < len(f.crosDUTs); i++ {
		client := *f.crosDUTs[i].client
		dut := f.crosDUTs[i].dut

		// Disable UWB
		err := uwb.CallAndCheckOK(client.Disable(ctx, &emptypb.Empty{}))
		if err != nil {
			s.Errorf("DUT %d error with Disable call in Teardown: %v", i, err)
		}

		// restart uwbd d-bus daemon
		err = restartUwbd(ctx, dut)
		if err != nil {
			s.Errorf("DUT %d error in restarting uwbd: %v", i, err)
		}
	}
}

func (f *mixedPeerRemoteFixture) Reset(ctx context.Context) error {
	for i := 0; i < len(f.crosDUTs); i++ {
		dut := f.crosDUTs[i].dut
		client := *f.crosDUTs[i].client

		// disable UWB
		err := uwb.CallAndCheckOK(client.Disable(ctx, &emptypb.Empty{}))
		if err != nil {
			return errors.Wrap(err, "Disable call error in fixture Reset")
		}

		// restart uwbd
		err = restartUwbd(ctx, dut)
		if err != nil {
			return err
		}

		// enable UWB
		err = uwb.CallAndCheckOK(client.Enable(ctx, &emptypb.Empty{}))
		if err != nil {
			return errors.Wrap(err, "Enable call error in fixture Reset")
		}
	}
	return nil
}
func (*mixedPeerRemoteFixture) PreTest(ctx context.Context, s *testing.FixtTestState)  {}
func (*mixedPeerRemoteFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}

func (f *mixedPeerRemoteFixture) setupADBPeers(ctx context.Context, s *testing.FixtState, companions []android.Companion) error {
	dutConn := s.DUT().Conn()

	// Launch ADB on the phone and connect adb server from phone to DUT. To make sure
	// that the DUT can always connect to the phone we forward several ports, the final
	// connection looks like: phone -> labstation -> drone -> DUT.
	port := 5555
	for i, c := range companions {
		testing.ContextLogf(ctx, "Android companion %d: associate %q serial %q model %q",
			i, c.AssociatedHostname, c.SerialNumber, c.ModelName)

		// Enable adb-over-tcp on the Android device with the serial number we are using this test run.
		s.Log("Setting up adb-over-tcp on the Android device")
		if err := f.labstation.CommandContext(ctx, "adb", "-s", c.SerialNumber, "tcpip", "5555").Run(ssh.DumpLogOnError); err != nil {
			errors.Wrap(err, "failed to enable adb-over-tcp on Android phone")
		}

		if err := f.labstation.CommandContext(ctx, "adb", "-s", c.SerialNumber, "wait-for-usb-device").Run(ssh.DumpLogOnError); err != nil {
			return errors.Wrap(err, "failed to wait for device state")
		}

		// Forward a port from the phone to the labstation.
		localPort := fmt.Sprintf("tcp:%d", port)
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if err := f.labstation.CommandContext(ctx, "adb", "-s", c.SerialNumber, "forward", localPort, "tcp:5555").Run(ssh.DumpLogOnError); err != nil {
				return err
			}

			// Check that forwarded port still exists.
			// The forward command may succeed but the port may immediately crash if adb-over-tcp isn't fully initialized.
			if out, err := f.labstation.CommandContext(ctx, "adb", "forward", "--list").Output(ssh.DumpLogOnError); err != nil {
				return err
			} else if !strings.Contains(string(out), c.SerialNumber) {
				return errors.New("cannot find newly forwarded port")
			}
			return nil
		}, &testing.PollOptions{
			Timeout:  30 * time.Second,
			Interval: 5 * time.Second,
		}); err != nil {
			return errors.Wrap(err, "failed to enable adb-over-tcp on Android phone")
		}

		onFwdError := func(tag string) func(error) {
			return func(err error) {
				testing.ContextLogf(ctx, "ssh forwarding error for %s: %q: %v", tag, c, err)
			}
		}

		// Forward the port from the labstation to the drone.
		// This is not strictly required, but the lab infra does not guarantee connectivity
		// between DUT and the labstation directly so lets forward it to be safe.
		labstationPort := fmt.Sprintf("127.0.0.1:%d", port)
		forwarder1, err := f.labstation.ForwardLocalToRemote("tcp", "localhost:", labstationPort, onFwdError("drone"))
		if err != nil {
			return errors.Wrap(err, "failed to forward local port to remote callbox manager")
		}

		// Forward the port now to the DUT so the DUT can locally access the phone.
		forwarder2, err := dutConn.ForwardRemoteToLocal("tcp", forwarder1.LocalAddr().String(), forwarder1.ListenAddr().String(), onFwdError("DUT"))
		if err != nil {
			return errors.Wrap(err, "failed to forward local port to remote callbox manager")
		}

		phone := &phonePeer{
			labToDrone: forwarder1,
			droneToDUT: forwarder2,
		}

		f.phonePeers = append(f.phonePeers, phone)
		port++
	}

	return nil
}

func (f *mixedPeerRemoteFixture) setupLabstation(ctx context.Context, s *testing.FixtState, hostname string) error {
	// Connect to the labstation. Use ProxyCommand so you don't have to port forward.
	sshOptions := &ssh.Options{
		KeyDir:       s.DUT().KeyDir(),
		KeyFile:      s.DUT().KeyFile(),
		ProxyCommand: s.DUT().ProxyCommand(),
	}

	if err := ssh.ParseTarget(hostname, sshOptions); err != nil {
		s.Fatal("Failed to parse labstation ssh target: ", err)
	}
	labstation, err := ssh.New(ctx, sshOptions)
	if err != nil {
		s.Fatal("Failed to connect to labstation phone host over ssh: ", err)
	}

	// Setup ADB on labstation.
	if err := labstation.CommandContext(ctx, "adb", "kill-server").Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed to kill any running adb server: ", err)
	}

	if err := labstation.CommandContext(ctx, "mkdir", "-p", "/run/arc/adb").Run(); err != nil {
		s.Fatal("Failed to make arc dir")
	}

	// Start with pre-configured vendor keys.
	cmdStrs := []string{
		"ADB_VENDOR_KEYS=/var/lib/android_keys",
		"adb start-server",
	}

	if err := labstation.CommandContext(ctx, "sh", "-c", strings.Join(cmdStrs, " ")).Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed to start adb with correct permissions: ", err)
	}

	// Print the existing devices attached to the labstation.
	out, err := labstation.CommandContext(ctx, "adb", "devices").Output()
	if err != nil {
		s.Fatal("Failed to run adb devices")
	}
	s.Log(string(out))

	f.labstation = labstation
	return nil
}

func remoteDUTSetUp(ctx context.Context, s *testing.FixtState, d *dut.DUT) (*us.UwbServiceClient, error) {
	// Restart UWBD D-bus service
	err := restartUwbd(ctx, d)
	if err != nil {
		return nil, err
	}

	cl, err := rpc.Dial(s.FixtContext(), d, s.RPCHint())
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to the RPC service on the DUT")
	}

	// Set UwbServiceClient
	svc := us.NewUwbServiceClient(cl.Conn)
	if _, err = svc.NewClient(ctx, &emptypb.Empty{}); err != nil {
		return nil, errors.Wrap(err, "could not start UWB D-Bus client")
	}

	// Call UWB enable
	err = uwb.CallAndCheckOK(svc.Enable(ctx, &emptypb.Empty{}))
	if err != nil {
		return nil, errors.Wrap(err, "enable call to dbus failed")
	}

	return &svc, nil
}

func restartUwbd(ctx context.Context, d *dut.DUT) error {
	cmd := d.Conn().CommandContext(ctx, "stop", "uwbd")
	err := cmd.Run()
	if err != nil {
		return errors.Wrap(err, "stop uwbd failed")
	}

	cmd = d.Conn().CommandContext(ctx, "start", "uwbd")
	err = cmd.Run()
	if err != nil {
		return errors.Wrap(err, "start uwbd failed")
	}
	return nil
}
