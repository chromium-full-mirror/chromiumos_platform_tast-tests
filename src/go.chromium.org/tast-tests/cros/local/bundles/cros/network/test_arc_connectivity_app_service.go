// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/network/systemproxy"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/testenv/proxy/mitmproxy"
	"go.chromium.org/tast-tests/cros/services/cros/network"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			network.RegisterTestArcConnectivityAppServiceServer(srv, &TestArcConnectivityAppService{state: s})
		},
	})
}

// TestArcConnectivityAppService implements the tast.cros.network.TestArcConnectivityAppService gRPC service.
type TestArcConnectivityAppService struct {
	state *testing.ServiceState
}

// RunTest verifies that Android apps can authenticate to remote proxy servers via the system-proxy daemon.
// This is the implementation of network.TestArcConnectivityAppService/RunTest gRPC.
func (s *TestArcConnectivityAppService) RunTest(ctx context.Context, req *empty.Empty) (_ *empty.Empty, retErr error) {
	tmpdir, err := ioutil.TempDir("", "fdms-")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create temp dir")
	}
	defer func() {
		if retErr != nil {
			if err := os.RemoveAll(tmpdir); err != nil {
				testing.ContextLogf(ctx, "Failed to delete %s: %v", tmpdir, err)
			}
			tmpdir = ""
		}
	}()

	// fakedms.New starts a background process that outlives the current context.
	fdms, err := fakedms.New(s.state.ServiceContext(), tmpdir) // NOLINT
	if err != nil {
		return nil, errors.Wrap(err, "failed to start FakeDMS")
	}
	defer func() {
		if retErr != nil {
			fdms.Stop(ctx)
		}
	}()

	// Check FakeDMS
	if err := fdms.Ping(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to ping FakeDMS")
	}

	pb := policy.NewBlob()

	if err := fdms.WritePolicyBlob(pb); err != nil {
		return nil, errors.Wrap(err, "failed to write policies to FakeDMS")
	}

	// Enroll the device.
	cr, err := chrome.New(
		ctx,
		chrome.FakeEnterpriseEnroll(chrome.Creds{User: "tast-user@managedchrome.com", Pass: "test0000"}),
		chrome.KeepEnrollment(), chrome.ARCEnabled(),
		chrome.FakeLogin(chrome.Creds{User: "tast-user@managedchrome.com", Pass: "test0000", GAIAID: "gaiaid"}),
		chrome.DMSPolicy(fdms.URL),
		chrome.ExtraArgs("--arc-availability=officially-supported"),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start Chrome")
	}

	const username = "testuser"
	const password = "testpwd"

	// Perform cleanup.
	if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
		return nil, errors.Wrap(err, "failed to clean up")
	}

	// Start an HTTP proxy instance on the DUT which requires username and password authentication.
	var opts []mitmproxy.Option

	opts = append(opts, mitmproxy.CustomOptions(fmt.Sprintf("proxyauth: %s:%s", username, password)),
		mitmproxy.CustomOptions(fmt.Sprintf("ignore_hosts: %s", "\n - .*")),
		mitmproxy.HealthCheck(false))

	proxy, err := mitmproxy.New(ctx, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create proxy server")
	}

	if err := proxy.Start(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to start proxy server")
	}

	defer proxy.Close(ctx)

	// Configure the proxy on the DUT via policy to point to the local proxy instance started via the `ProxyService`.
	proxyModePolicy := &policy.ProxyMode{Val: "fixed_servers"}
	proxyServerPolicy := &policy.ProxyServer{Val: fmt.Sprintf("http://%s", proxy.ProxyAddress())}

	// Start system-proxy and configure it with the credentials of the local proxy instance.
	systemProxySettingsPolicy := &policy.SystemProxySettings{
		Val: &policy.SystemProxySettingsValue{
			SystemProxyEnabled:           true,
			SystemServicesUsername:       username,
			SystemServicesPassword:       password,
			PolicyCredentialsAuthSchemes: []string{},
		}}

	arcEnabledPolicy := &policy.ArcEnabled{Val: true}

	// Update policies.
	if err := policyutil.ServeAndRefresh(ctx, fdms, cr, []policy.Policy{proxyModePolicy, proxyServerPolicy, systemProxySettingsPolicy, arcEnabledPolicy}); err != nil {
		return nil, errors.Wrap(err, "failed to update policies")
	}

	a, err := arc.New(ctx, tmpdir, cr.NormalizedUser())
	if err != nil {
		return nil, errors.Wrap(err, "failed to start arc")
	}
	defer a.Close(ctx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create test API connection")
	}

	actualUsedProxy, err := systemproxy.RunArcConnectivityApp(ctx, a, tconn, "https://www.google.com/" /*useSystemProxy=*/, true, username, password)
	if err != nil {
		return nil, errors.Wrap(err, "failed to test app connectivity")
	}

	// System-proxy has an address in the 100.115.92.0/24 subnet (assigned by patchpanel) and listens on port 3128.
	expectedProxy := regexp.MustCompile("100.115.92.[0-9]+:3128")
	if !expectedProxy.Match([]byte(actualUsedProxy)) {
		return nil, errors.Wrapf(err, "the ARC++ app is not using the system-proxy daemon: %s", actualUsedProxy)
	}
	return &empty.Empty{}, nil
}
