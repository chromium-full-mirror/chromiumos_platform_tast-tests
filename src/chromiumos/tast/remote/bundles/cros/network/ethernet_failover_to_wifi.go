// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/remote/wificell"
	"chromiumos/tast/services/cros/ui"
	"chromiumos/tast/services/cros/wifi"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         EthernetFailoverToWifi,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify ChromeOS is able to transition from LAN connection to a Wifi connection on OOBE, user and guest session",
		Contacts: []string{
			"cros-connectivity@google.com",
			"cros-conn-test-team@google.com",
			"cienet-development@googlegroups.com",
			"sun.tsai@cienet.com",
		},
		BugComponent: "b:1131775", // ChromeOS > Software > System Services > Connectivity
		Attr:         []string{"group:wificell", "wificell_e2e_unstable"},
		ServiceDeps: []string{
			wificell.TFServiceName,
			"tast.cros.browser.ChromeService",
			"tast.cros.wifi.WifiService",
		},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "wificellFixt",
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		Params: []testing.Param{
			{
				Name: "normal_user",
				Val:  &ui.NewRequest{},
			}, {
				Name: "guest_user",
				Val: &ui.NewRequest{
					LoginMode: ui.LoginMode_LOGIN_MODE_GUEST_LOGIN,
				},
			}, {
				Name: "oobe",
				Val: &ui.NewRequest{
					LoginMode: ui.LoginMode_LOGIN_MODE_NO_LOGIN,
				},
			},
		},
		Timeout: 5 * time.Minute,
	})
}

// EthernetFailoverToWifi verifies ChromeOS is able to transition from LAN connection to a Wifi connection on OOBE, user and guest session.
func EthernetFailoverToWifi(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)

	ap, err := tf.DefaultOpenNetworkAP(ctx)
	if err != nil {
		s.Fatal("Failed to configure the AP: ", err)
	}
	defer tf.DeconfigAP(ctx, ap)
	ctx, cancel := tf.ReserveForDeconfigAP(ctx, ap)
	defer cancel()

	rpcClient := tf.DUTRPC(wificell.DefaultDUT)
	wifiClient := tf.DUTWifiClient(wificell.DefaultDUT)
	cr := ui.NewChromeServiceClient(rpcClient.Conn)
	wifiSvc := wifi.NewWifiServiceClient(rpcClient.Conn)

	cleanupCtx := ctx
	ctx, cancel = ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	newCrRequest := s.Param().(*ui.NewRequest)
	if newCrRequest.LoginMode == ui.LoginMode_LOGIN_MODE_NO_LOGIN {
		newCrRequest.SigninProfileTestExtensionId = s.RequiredVar("ui.signinProfileTestExtensionManifestKey")
	}
	if _, err := cr.New(ctx, newCrRequest); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx, &emptypb.Empty{})

	if _, err := wifiSvc.JoinWifiFromQuickSettings(ctx, &wifi.JoinWifiRequest{
		Ssid:     ap.Config().SSID,
		Security: &wifi.JoinWifiRequest_None{},
	}); err != nil {
		s.Fatalf("Failed to connect to WiFi %q: %v", ap.Config().SSID, err)
	}
	defer tf.CleanDisconnectDUTFromWifi(cleanupCtx, wificell.DefaultDUT)

	// Verify the network is transitioned from ethernet to WiFi.
	if err := wifiClient.TransitionFromEthernetAndRecover(ctx, ap.Config().SSID); err != nil {
		s.Fatal("Failed to test if the network is transitioned from ethernet to WiFi: ", err)
	}
}
