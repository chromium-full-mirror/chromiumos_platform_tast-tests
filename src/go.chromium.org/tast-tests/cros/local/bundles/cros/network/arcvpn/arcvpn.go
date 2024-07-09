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

// InstallAndPreAuthorizeARCVPN installs ARC VPN app and pre-authorizes the
// package so that the Android system doesn't create a pop UI asking the user
// to authorize the VPN app. The caller is responsible to call the returned
// cleanup function to uninstall the app.
func InstallAndPreAuthorizeARCVPN(ctx context.Context, a *arc.ARC) (func(context.Context), error) {
	testing.ContextLog(ctx, "Installing ArcVpnTest.apk")
	if err := a.Install(ctx, arc.APKPath(VPNTestAppAPK)); err != nil {
		return nil, errors.Wrap(err, "failed to install app")
	}

	testing.ContextLog(ctx, "Preauthorizing ArcVpnTest")
	if _, err := a.Command(ctx, "dumpsys", "wifi", "authorize-vpn", VPNTestAppPkg).Output(testexec.DumpLogOnError); err != nil {
		testing.ContextLog(ctx, "Failed to preauthorize ArcVpnTest, uninstalling")
		if err := a.Uninstall(ctx, VPNTestAppPkg); err != nil {
			testing.ContextLog(ctx, "Failed to uninstall ArcVpnTest.apk: ", err)
		}
		return nil, errors.Wrap(err, "failed to execute authorize-vpn command")
	}
	return func(ctx context.Context) {
		testing.ContextLog(ctx, "Uninstalling ArcVpnTest.apk")
		if err := a.Uninstall(ctx, VPNTestAppPkg); err != nil {
			testing.ContextLog(ctx, "Failed to uninstall ArcVpnTest.apk: ", err)
		}
	}, nil
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
	return broadcastLaunchVPNIntent(ctx, a)
}

// StartARCVPNAndSetupSocket starts the ARC test vpn app by broadcasting an
// intent that the ArcVpnTestApp
// BroadcastReceiver receives, and meanwhile sets up a socket using:
// (1) given address and port of the remote peer we want to connect to
// (2) protocol of the socket (One of l4server.TCP* or l4server.UDP*)
// (3) name of the interface in ARC we want to use to setup the socket
// This methods combines starting service and setting up socket since a delay
// between the service starts and the first intent can be received by the
// service is observed. Combining two can make sure socket is set up when
// service is ready.
func StartARCVPNAndSetupSocket(ctx context.Context, a *arc.ARC, family l4server.Family, ifname, address string, port int) error {
	if family == l4server.TCP6 || family == l4server.TCP4 {
		family = l4server.TCP
	}
	if family == l4server.UDP4 || family == l4server.UDP6 {
		family = l4server.UDP
	}

	args := []string{
		"--es", "sockproto", family.String(),
		"--es", "sockinterface", ifname,
		"--es", "sockaddress", address,
		"--ei", "sockport", strconv.Itoa(port),
	}
	return broadcastLaunchVPNIntent(ctx, a, args...)
}

// StartARCVPNWithToyServer starts the ARC test vpn app with connecting to
// vpnServer, by broadcasting an intent that the ArcVpnTestApp BroadcastReceiver
// receives.
func StartARCVPNWithToyServer(ctx context.Context, a *arc.ARC, arcIfname string, vpnServer *vpn.Server) error {
	args := []string{
		"--es", "interface", arcIfname,
		"--es", "address", vpnServer.UnderlayIP,
		"--ei", "port", strconv.Itoa(vpn.ToyVPNServerPort),
		"--es", "overlay_address", vpnServer.Config.GetOverlayConfig().ClientIPv4,
		"--es", "dns_server", vpnServer.OverlayIPv4,
		"--ei", "mtu", strconv.Itoa(vpnServer.Config.MTU),
	}
	return broadcastLaunchVPNIntent(ctx, a, args...)
}

func broadcastLaunchVPNIntent(ctx context.Context, a *arc.ARC, extraArgs ...string) error {
	// Only components with android.permission.BIND_VPN_SERVICE can bind to
	// VPNs, so we can't directly start the service from a non-root shell.
	// Instead, have a broadcast receiver ask that the system start the VPN.
	args := []string{"--include-stopped-packages", "--receiver-include-background"}
	args = append(args, extraArgs...)
	if _, err := a.BroadcastIntent(ctx, VPNTestAppBroadcast, args...); err != nil {
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

// SendMessage sends out a message via the latest setup socket.
func SendMessage(ctx context.Context, a *arc.ARC, message string) error {
	if _, err := a.BroadcastIntent(ctx,
		VPNTestAppSendMessageIntent,
		"--es", "message", message); err != nil {
		return errors.Wrap(err, "send message failed")
	}
	return nil
}
