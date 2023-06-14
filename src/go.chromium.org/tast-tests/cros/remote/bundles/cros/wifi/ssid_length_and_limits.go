// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type ssidSmokeTestParam struct {
	ssids []string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         SSIDLengthAndLimits,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that ChromeOS properly handles the SSID character length and limits",
		Contacts: []string{
			"cros-connectivity@google.com",
			"chromeos-connectivity-engprod@google.com",
			"kinwang.lao@cienet.com",
			"cienet-development@googlegroups.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent: "b:1131912", // ChromeOS > Software > System Services > Connectivity > WiFi
		Attr:         []string{"group:wificell", "wificell_e2e_unstable"},
		ServiceDeps: []string{
			wificell.ShillServiceName,
			"tast.cros.browser.ChromeService",
			"tast.cros.wifi.WifiService",
		},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{
			{
				Name: "symbols",
				Val: ssidSmokeTestParam{
					ssids: []string{symbols(1), symbols(32)},
				},
			},
		},
		Fixture: "wificellFixt",
		Timeout: 3 * time.Minute,
	})
}

// SSIDLengthAndLimits verifies that ChromeOS properly handles the SSID character length and limits.
func SSIDLengthAndLimits(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tf := s.FixtValue().(*wificell.TestFixture)
	rpcClient := tf.DUTRPC(wificell.DefaultDUT)
	crSvc := ui.NewChromeServiceClient(rpcClient.Conn)
	if _, err := crSvc.New(ctx, &ui.NewRequest{}); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer crSvc.Close(cleanupCtx, &emptypb.Empty{})

	param := s.Param().(ssidSmokeTestParam)
	for _, ssid := range param.ssids {
		s.Run(ctx, ssid, func(ctx context.Context, s *testing.State) {
			opts := append(wificell.DefaultOpenNetworkAPOptions(), hostapd.SSID(ssid))
			ap, err := tf.ConfigureAP(ctx, opts, nil)
			if err != nil {
				s.Fatal("Failed to configure the AP: ", err)
			}
			cleanupAPCtx := ctx
			defer tf.DeconfigAP(cleanupAPCtx, ap)
			ctx, cancel := tf.ReserveForDeconfigAP(ctx, ap)
			defer cancel()

			s.Log("SSID under test: ", ap.Config().SSID)
			wifiSvc := wifi.NewWifiServiceClient(rpcClient.Conn)
			if _, err := wifiSvc.JoinWifiFromQuickSettings(ctx, &wifi.JoinWifiRequest{
				Ssid:     ap.Config().SSID,
				Security: &wifi.JoinWifiRequest_None{},
			}); err != nil {
				s.Fatal("Failed to join WiFi from Quick Settings: ", err)
			}
			cleanupCtx = ctx
			ctx, cancel = tf.ReserveForDisconnect(ctx)
			defer cancel()
			defer tf.CleanDisconnectDUTFromWifi(cleanupCtx, wificell.DefaultDUT)

			wifiClient := tf.DUTWifiClient(wificell.DefaultDUT)
			if err := wifiClient.WaitForConnected(ctx, ap.Config().SSID, true /* expectedValue */); err != nil {
				s.Fatal("Failed to wait for WiFi is connected: ", err)
			}
		})
	}
}

// symbols returns a string of ASCII "symbols" (normal letters A-Z, a-z and 0-9 are not included) with specified length.
func symbols(length int) string {
	const symbols = `!"#$%&'()*+,-./:;<=>?@[\]^_` + "`{|}~"

	s := strings.Repeat(symbols, 1+length/len(symbols))
	return s[:length]
}
