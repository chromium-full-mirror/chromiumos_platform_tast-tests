// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package networkui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           EthernetFailoverToWifi,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Verify ChromeOS is able to transition from LAN connection to a Wifi connection on OOBE, user and guest session",
		Contacts: []string{
			"cros-connectivity@google.com",
			"chromeos-connectivity-engprod@google.com",
			"shijinabraham@google.com",
			"chadduffin@chromium.org",
		},
		BugComponent: "b:1318544", // ChromeOS > Software > System Services > Connectivity > General
		Attr:         []string{"group:wificell", "wificell_e2e"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.PeripheralWifiStateWorking},
		ServiceDeps: []string{
			wificell.ShillServiceName,
			"tast.cros.browser.ChromeService",
			"tast.cros.wifi.WifiService",
		},
		SoftwareDeps: []string{"chrome"},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesNone),
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
