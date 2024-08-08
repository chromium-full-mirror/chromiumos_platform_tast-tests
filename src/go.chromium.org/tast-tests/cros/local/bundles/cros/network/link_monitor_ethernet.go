// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/golang/protobuf/proto"

	pppb "go.chromium.org/chromiumos/system_api/patchpanel_proto"
	"go.chromium.org/tast-tests/cros/common/crypto/certificate"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/captiveportalconsts"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/network/testhooks"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/certs"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LinkMonitorEthernet,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies gateway reachability fail changes network to no connectivity",
		Contacts:     []string{"cros-networking@google.com", "ningyuan@google.com"},
		// ChromeOS > Platform > System > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		// TODO(b/261383020): Promote to group:network after test is stable.
		Attr:    []string{"group:mainline", "informational", "group:hw_agnostic"},
		Timeout: 10 * time.Minute,
	})
}

// LinkMonitorEthernet checks the following events occur when
// the gateway is unreachable on an ethernet network: link monitor in patchpanel
// signals NeighborReachabilityEvent DBus signal. Shill changes the network
// state to no connectivity after a network validation after receiving the
// signal. The gateway unreachable is triggered by removing ip address on the
// gateway in the virtualnet environment.
func LinkMonitorEthernet(ctx context.Context, s *testing.State) {
	// Reserve some time for cleanup code.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 20*time.Second)
	defer cancel()

	hookEnv, err := testhooks.RunNetworkTestHooks(ctx,
		testhooks.NewSaveNetLogHook(),
		testhooks.NewTcpdumpHook(),
		testhooks.NewDumpHostOnFailureHook(),
	)
	if err != nil {
		s.Fatal("Failed to run network test hooks: ", err)
	}
	s.AttachErrorHandlers(hookEnv.OnErrorHandler, hookEnv.OnFatalHandler)
	defer hookEnv.TearDownWithLogFailures(cleanupCtx, s.HasError)

	shillManager, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create shill client: ", err)
	}

	// Configure shill properties and set up https server in virtualnet for
	// portal detection.
	originalPortalURLs, err := shillManager.GetAndSetProperty(ctx, shillconst.ManagerPropertyPortalFallbackHTTPSURLs, captiveportalconsts.HTTPSPortalURL)
	if err != nil {
		s.Fatal("Failed to override portal https urls: ", err)
	}
	defer func() {
		if err := shillManager.SetProperty(cleanupCtx, shillconst.ManagerPropertyPortalFallbackHTTPSURLs, originalPortalURLs); err != nil {
			s.Error("Failed to revert portal fallback https urls: ", err)
		}
	}()

	httpsCerts := certs.New(certs.SSLCrtPath, certificate.TestCert3())
	cleanupCerts, err := httpsCerts.InstallTestCerts(ctx)
	if err != nil {
		s.Fatal("Failed to setup certificates: ", err)
	}
	defer cleanupCerts(cleanupCtx)

	opts := virtualnet.EnvOptions{
		EnableDHCP:                 true,
		Priority:                   5,
		EnableDNS:                  true,
		HTTPSServerResponseHandler: captiveportalconsts.NoContentHandler,
		HTTPServerResponseHandler:  captiveportalconsts.NoContentHandler,
		HTTPSCerts:                 httpsCerts,
	}

	pool := subnet.NewPool()
	svc, router, err := virtualnet.CreateRouterEnv(ctx, shillManager, pool, opts)
	if err != nil {
		s.Fatalf("Failed to create %s: %s", router.NetNSName, err)
	}
	defer func() {
		if err := router.Cleanup(cleanupCtx); err != nil {
			s.Fatalf("Failed to clean %s: %s", router.NetNSName, err)
		}
	}()

	addr, err := router.WaitForVethInAddrs(ctx, true, false)
	if err != nil {
		s.Fatalf("Cannot get IP address of %s: %s", router.NetNSName, err)
	}
	if err := ping.ExpectPingSuccessWithTimeout(ctx, addr.IPv4Addr.String(), "root", 10*time.Second); err != nil {
		s.Fatalf("Failed to ping gateway %s on host: %s", addr.IPv4Addr.String(), err)
	}

	if err := svc.WaitForProperty(ctx, shillconst.ServicePropertyState, shillconst.ServiceStateOnline, 10*time.Second); err != nil {
		s.Fatalf("Failed to verify shill service state of %s change to online: %s", router.NetNSName, err)
	}

	// Watch for NeighborReachabilityEventSignal.
	spec := dbusutil.MatchSpec{
		Type:      "signal",
		Path:      dbus.ObjectPath("/org/chromium/PatchPanel"),
		Interface: "org.chromium.PatchPanel",
		Member:    "NeighborReachabilityEvent",
	}
	watcher, err := dbusutil.NewSignalWatcherForSystemBus(ctx, spec)
	if err != nil {
		s.Fatal("Failed to create DBus signal watcher: ", err)
	}
	defer func() {
		if err := watcher.Close(ctx); err != nil {
			s.Fatal("Failed to close DBus signal watcher: ", err)
		}
	}()

	s.Logf("Remove IP address from %s, expect link state change in about a minute", router.NetNSName)
	if err := router.RunWithoutChroot(ctx, "ip", "addr", "del", addr.IPv4Addr.String(), "dev", router.VethInName); err != nil {
		s.Fatalf("Remove IP address from %s failed: %s", router.NetNSName, err)
	}

	// patchpanel::NeighborLinkMonitor probes neighbor states every 60 seconds.
	linkTimeoutCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	select {
	case sig := <-watcher.Signals:
		// Takes the signal body, and interpret as patchpanel protobuf binary.
		body := sig.Body[0].([]byte)
		sigResult := &pppb.NeighborReachabilityEventSignal{}
		if err := proto.Unmarshal(body, sigResult); err != nil {
			s.Fatalf("Cannot parse reachability signal: %s", err)
		}
		s.Logf("Detected NeighborReachabilityEventSignal, role: %s, type: %s", sigResult.GetRole().String(), sigResult.GetType().String())
		if sigResult.GetType() == pppb.NeighborReachabilityEventSignal_FAILED {
			break
		}

	case <-linkTimeoutCtx.Done():
		s.Fatal("Timeout waiting for NeighborReachabilityEventSignal")
	}

	// Wait for portal detection failure to trigger network state change.
	if err := svc.WaitForProperty(ctx, shillconst.ServicePropertyState, shillconst.ServiceStateNoConnectivity, 20*time.Second); err != nil {
		s.Fatal("Failed to wait for service to no-connectivity: ", err)
	}
}
