// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/services/cros/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type handleAndSanitizeWifiSSIDTestParam struct {
	ssids                 []string
	verificationOnJoining func(context.Context, *grpc.ClientConn, string) error
}

const handleAndSanitizeWifiSSIDTimeout = 3 * time.Minute

func init() {
	testing.AddTest(&testing.Test{
		Func:           HandleAndSanitizeWifiSSID,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Verify that ChromeOS properly handles and sanitizes the SSIDs of WiFi networks",
		Contacts: []string{
			"cros-connectivity@google.com",
			"chromeos-connectivity-engprod@google.com",
			"shijinabraham@google.com",
			"chadduffin@chromium.org",
		},
		BugComponent: "b:1131912", // ChromeOS > Software > System Services > Connectivity > WiFi
		Attr:         []string{"group:wificell", "wificell_e2e"},
		ServiceDeps: []string{
			wificell.ShillServiceName,
			"tast.cros.browser.ChromeService",
			"tast.cros.wifi.WifiService",
			"tast.cros.ui.AutomationService",
			"tast.cros.ui.ChromeUIService",
			"tast.cros.chrome.uiauto.ossettings.OsSettingsService",
		},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{
			{
				Name: "symbols",
				Val: handleAndSanitizeWifiSSIDTestParam{
					ssids: []string{symbols(1), symbols(32)},
				},
				// 2 SSIDs under test.
				Timeout: 2 * handleAndSanitizeWifiSSIDTimeout,
			}, {
				Name: "js_injection",
				Val: handleAndSanitizeWifiSSIDTestParam{
					ssids: []string{
						`+ADw-SCRIPT+AD4-alert(1);`,
						`';alert(3)//`,
						`\';alert(4)//`,
						`"";alert(5)//`,
						`\"";alert(6)//`,
						// TODO(b/286254915): Add the following SSID once the issue is resolved.
						// `<img src=x onerror=alert(2)>`
						// `"">'><SCRIPT>alert(7)</SCRIPT>`
					},
					verificationOnJoining: verifyJSInjectionSafe,
				},
				// 5 SSIDs under test.
				Timeout: 5 * handleAndSanitizeWifiSSIDTimeout,
			},
		},
		Fixture: wificell.FixtureID(wificell.TFFeaturesNone),
	})
}

// HandleAndSanitizeWifiSSID verifies that ChromeOS properly handles and sanitizes the SSIDs of WiFi networks.
func HandleAndSanitizeWifiSSID(ctx context.Context, s *testing.State) {
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

	param := s.Param().(handleAndSanitizeWifiSSIDTestParam)
	for _, ssid := range param.ssids {
		s.Run(ctx, "SSID under test: "+ssid, func(ctx context.Context, s *testing.State) {
			opts := append(wificell.DefaultOpenNetworkAPOptions(), hostapd.SSID(ssid))
			ap, err := tf.ConfigureAP(ctx, opts, nil)
			if err != nil {
				s.Fatal("Failed to configure the AP: ", err)
			}
			cleanupAPCtx := ctx
			defer tf.DeconfigAP(cleanupAPCtx, ap)
			ctx, cancel := tf.ReserveForDeconfigAP(ctx, ap)
			defer cancel()

			if param.verificationOnJoining != nil {
				if err := param.verificationOnJoining(ctx, rpcClient.Conn, ap.Config().SSID); err != nil {
					s.Fatal("Test verification failed: ", err)
				}
			}

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

// verifyJSInjectionSafe verifies there is no alert window generated by JavaScript shows up on WiFi page
// and the name of the network displays properly and doesn't attempt to connect to the network.
func verifyJSInjectionSafe(ctx context.Context, conn *grpc.ClientConn, ssid string) (retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	ossettingsSvc := ossettings.NewOsSettingsServiceClient(conn)
	if _, err := ossettingsSvc.LaunchAtWifiPage(ctx, &emptypb.Empty{}); err != nil {
		return errors.Wrap(err, "failed to launch OS-settings at WiFi page")
	}
	defer func(ctx context.Context) {
		if retErr != nil {
			faillogSvc := ui.NewChromeUIServiceClient(conn)
			faillogSvc.DumpUITreeWithScreenshotToFile(ctx, &ui.DumpUITreeWithScreenshotToFileRequest{
				FilePrefix: "verify_js_injection_safe",
			})
		}
		ossettingsSvc.Close(ctx, &emptypb.Empty{})
	}(cleanupCtx)

	uiSvc := ui.NewAutomationServiceClient(conn)
	alertDialog := ui.Node().Role(ui.Role_ROLE_WINDOW).HasClass("JavaScriptTabModalDialogViewViews").Nth(0)
	if _, err := uiSvc.EnsureGone(ctx, &ui.EnsureGoneRequest{
		Finder: alertDialog.Finder(),
	}); err != nil {
		return errors.Wrap(err, "failed to verify no alert window")
	}

	wifiFinder := ui.Node().NameRegex(fmt.Sprintf(`^Network (\d)+ of (\d)+, %s`, regexp.QuoteMeta(ssid))).HasClass("layout")
	// It could take a longer time to wait when the device is not connected.
	if _, err := uiSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{
		Finder:  wifiFinder.Finder(),
		Timeout: &durationpb.Duration{Seconds: 60},
	}); err != nil {
		return errors.Wrap(err, "failed to wait until WiFi node exists")
	}

	// Verify the network does not attempt to be connected or already connected.
	connectNode := ui.Node().NameRegex("^Connect(ed|ing)").Role(ui.Role_ROLE_STATIC_TEXT).Ancestor(wifiFinder.Finder())
	if _, err := uiSvc.EnsureGone(ctx, &ui.EnsureGoneRequest{
		Finder:  connectNode.Finder(),
		Timeout: &durationpb.Duration{Seconds: 5},
	}); err != nil {
		return errors.Wrap(err, "failed to verify WiFi is not connected")
	}
	return nil
}
