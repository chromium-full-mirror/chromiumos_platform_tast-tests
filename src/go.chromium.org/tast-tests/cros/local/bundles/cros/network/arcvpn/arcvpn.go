// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package arcvpn interacts with the ARC-side fake VPN.
package arcvpn

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/l4server"
	"go.chromium.org/tast-tests/cros/local/network/vpn"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// These need to stay in sync with /vendor/google_arc/packages/system/ArcHostVpn
const (
	FacadeVPNPkg = "org.chromium.arc.hostvpn"
	FacadeVPNSvc = "org.chromium.arc.hostvpn.ArcHostVpnService"
)

// These need to stay in sync with
// //platform/tast-tests/android/ArcVpnTest/src/org/chromium/arc/testapp/arcvpn/ArcTestVpnService.java
const (
	VPNTestAppAPK               = "ArcVpnTest.apk"
	VPNTestAppPkg               = "org.chromium.arc.testapp.arcvpn"
	VPNTestAppSvc               = "org.chromium.arc.testapp.arcvpn.ArcTestVpnService"
	VPNTestAppBroadcast         = "org.chromium.arc.testapp.arcvpn.LAUNCH_VPN"
	VPNTestAppSetupSocketIntent = "org.chromium.arc.testapp.arcvpn.SETUP_SOCKET"
	VPNTestAppSendMessageIntent = "org.chromium.arc.testapp.arcvpn.SEND_MESSAGE"
	TunIP                       = "192.168.2.2"
)

// SetUpHostVPN create the host VPN server, but does not initiate a connection.
// The returned vpn.Connection is immediately ready for Connect() to be called
// on it. The caller should call Cleanup() on the returned Connection after the
// test is done.
func SetUpHostVPN(ctx context.Context, vpnType vpn.Type, opts ...vpn.Option) (*vpn.Connection, error) {
	opts = append(opts, vpn.WithoutAutoConnect())
	return vpn.StartConnection(ctx, nil, vpnType, opts...)
}

// SetARCVPNEnabled flips the flag in the current running ARC instance. If running multiple tests
// within the same ARC instance, it's recommended to cleanup by flipping the flag back to the
// expected default state afterwards. Since no state is persisted, new ARC instances will initialize
// with the default state.
func SetARCVPNEnabled(ctx context.Context, a *arc.ARC, enabled bool) error {
	testing.ContextLogf(ctx, "Setting arc-host-vpn flag to %t", enabled)
	cmd := a.Command(ctx, "dumpsys", "wifi", "set-arc-host-vpn", fmt.Sprintf("%t", enabled))
	o, err := cmd.Output(testexec.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "failed to execute 'set-arc-host-vpn' commmand")
	}

	if !strings.Contains(string(o), "sEnableArcHostVpnAdbFlag="+fmt.Sprintf("%t", enabled)) {
		return errors.New("unable to set sEnableArcHostVpnAdbFlag to " + fmt.Sprintf("%t", enabled))
	}
	return nil
}

// StartARCVPN starts the ARC test vpn app by broadcasting an intent that the ArcVpnTestApp
// BroadcastReceiver receives.
func StartARCVPN(ctx context.Context, a *arc.ARC) error {
	// Only components with android.permission.BIND_VPN_SERVICE can bind to
	// VPNs, so we can't directly start the service from a non-root shell.
	// Instead, have a broadcast receiver ask that the system start the VPN.
	if _, err := a.BroadcastIntent(ctx, VPNTestAppBroadcast, "--include-stopped-packages", "--receiver-include-background"); err != nil {
		return errors.Wrapf(err, "failed to send %s intent", VPNTestAppBroadcast)
	}
	return nil
}

// WaitForARCServiceState checks if the Android service is running in the `expectedRunning` state.
func WaitForARCServiceState(ctx context.Context, a *arc.ARC, pkg, svc string, expectedRunning bool) error {
	testing.ContextLogf(ctx, "Check the state of %s/%s", pkg, svc)

	// Given a service like "com.hello.world", we want to use the last "."-delimited element
	// which is the actual service name, in this case "world".
	svcSplits := strings.Split(svc, ".")
	svcName := svcSplits[len(svcSplits)-1]

	// Poll since it might take some time for the service to start/stop.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		cmd := a.Command(ctx, "dumpsys", "activity", "services", pkg+"/"+svc)
		o, err := cmd.Output(testexec.DumpLogOnError)
		if err != nil {
			return errors.Wrap(err, "failed to execute 'dumpsys activity services' commmand")
		}

		// Use raw string so we can directly use backslashes
		// example outputs we want to match on:
		//   ServiceRecord{27b48e1 u0 org.chromium.arc.hostvpn/.ArcHostVpnService}
		//   ServiceRecord{b1145f6 u0 com.android.vending/com.google.android.finsky.setup.PlaySetupServiceV2}
		matched, matchErr := regexp.Match(`ServiceRecord\{[0-9a-z]+ u[0-9]+ `+pkg+`\/.+`+svcName, o)
		if matched != expectedRunning || matchErr != nil {
			if expectedRunning {
				return errors.Wrap(matchErr, "expected, but didn't find ServiceRecord")
			}
			return errors.Wrap(matchErr, "didn't expect, but found ServiceRecord")
		}

		return nil
	}, &testing.PollOptions{Timeout: 15 * time.Second}); err != nil {
		return errors.Wrapf(err, "service not in expected running state of %t", expectedRunning)
	}
	return nil
}

// ForceStopARCVPN will forcibly terminate ArcHostVpnService, which also appears like a crash. This
// doesn't exercise normal ArcNetworkService->ArcHostVpnService service disconnection flows. If
// ArcHostVpnService isn't running, then this is a no-op.
func ForceStopARCVPN(ctx context.Context, a *arc.ARC) error {
	testing.ContextLog(ctx, "Sending am force-stop to ArcHostVpnService")
	cmd := a.Command(ctx, "am", "force-stop", FacadeVPNPkg)
	if err := cmd.Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to execute 'am force-stop' commmand")
	}

	if err := WaitForARCServiceState(ctx, a, FacadeVPNPkg, FacadeVPNSvc, false); err != nil {
		return errors.Wrapf(err, "failed to stop %s", FacadeVPNSvc)
	}
	return nil
}

// SetupSocket sets up a socket using given address and port of the remote peer
// we want to connect to, and protocol of the socket. This setup only works
// with WiFi and only when there is one WiFi network in ARC. Do not support
// Ethernet now because it's common that there are multiple Ethernet networks
// on DUT with virtualnet package, and we don't have a easy way to determine
// which Ethernet we should use in ARC now.
// When called multiple times in one test, the older socket will be replaced by
// newly setup socket for sending messages.
func SetupSocket(ctx context.Context, a *arc.ARC, family l4server.Family, address string, port int) error {
	if family == l4server.TCP6 || family == l4server.TCP4 {
		family = l4server.TCP
	}
	if family == l4server.UDP4 || family == l4server.UDP6 {
		family = l4server.UDP
	}

	if _, err := a.BroadcastIntent(ctx,
		VPNTestAppSetupSocketIntent,
		"--es", "proto", family.String(),
		"--es", "address", address,
		"--ei", "port", strconv.Itoa(port)); err != nil {
		return errors.Wrap(err, "setup socket failed")
	}
	return nil
}

// SendMessage sends out a message via the latest setup socket.
func SendMessage(ctx context.Context, a *arc.ARC, message string) error {
	if _, err := a.BroadcastIntent(ctx,
		VPNTestAppSendMessageIntent,
		"--es", "message", message); err != nil {
		return errors.Wrap(err, "send message failed")
	}
	return nil
}
