// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"

	"chromiumos/tast/common/shillconst"
	"chromiumos/tast/common/wifi/security/wpa"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/remote/bundles/cros/wifi/wifiutil"
	"chromiumos/tast/remote/wificell"
	"chromiumos/tast/remote/wificell/hostapd"
	"chromiumos/tast/services/cros/chrome/uiauto/ossettings"
	"chromiumos/tast/services/cros/ui"
	"chromiumos/tast/testing"
)

type passphraseTestCase struct {
	name              string
	passphraseLength  int
	genPassphrase     func(int) (string, error)
	verifications     func(context.Context, *grpc.ClientConn, *wificell.WifiClient, string) error
	extraVerification func(context.Context, *grpc.ClientConn, *wificell.WifiClient, string) error
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         HandlePassphrases,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify ChromeOS handles passphrases gracefully and displays error messages or UI prompts correctly",
		Contacts: []string{
			"cj.tsai@cienet.com",
			"cienet-development@googlegroups.com",
			"chromeos-wifi-champs@google.com",
		},
		BugComponent: "b:1131912",
		Attr:         []string{"group:wificell", "wificell_e2e_unstable"},
		ServiceDeps: append(
			wifiutil.JoinWifiServiceNames,
			wificell.TFServiceName,
			"tast.cros.browser.ChromeService",
			"tast.cros.ui.AutomationService",
			"tast.cros.chrome.uiauto.ossettings.OsSettingsService",
			wifiutil.FaillogServiceName,
		),

		SoftwareDeps: []string{"chrome"},
		Fixture:      "wificellFixt",
		Timeout:      6 * time.Minute,
	})
}

// HandlePassphrases verifies ChromeOS handles passphrases (including correct, incorrect-valid and incorrect-invalid ones)
// gracefully and displays error messages or UI prompts correctly.
func HandlePassphrases(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)
	rpcClient := tf.DUTRPC(wificell.DefaultDUT)
	wifiSvc := tf.DUTWifiClient(wificell.DefaultDUT)
	correctPassphrase := "WiFi_t1e2s3t4"
	opts := []hostapd.Option{
		hostapd.Channel(1),
		hostapd.SSID(hostapd.RandomSSID("Hidden_WiFi_")),
		hostapd.Mode(hostapd.Mode80211g),
		hostapd.Hidden(),
	}
	securityConfig := wpa.NewConfigFactory(correctPassphrase, wpa.Mode(wpa.ModePureWPA), wpa.Ciphers(wpa.CipherTKIP, wpa.CipherCCMP))

	ap, err := tf.ConfigureAP(ctx, opts, securityConfig)
	if err != nil {
		s.Fatal("Failed to configure the AP: ", err)
	}
	defer func(ctx context.Context) {
		if err := tf.DeconfigAP(ctx, ap); err != nil {
			s.Error("Failed to deconfig the AP: ", err)
		}
	}(ctx)

	ctx, cancel := tf.ReserveForDeconfigAP(ctx, ap)
	defer cancel()

	// cleanupCtx is the context with time reserved, used for cleaning up resources other than the AP.
	cleanupCtx := ctx
	ctx, cancel = ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	crSvc := ui.NewChromeServiceClient(rpcClient.Conn)
	if _, err := crSvc.New(ctx, &ui.NewRequest{}); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer crSvc.Close(cleanupCtx, &emptypb.Empty{})

	// Ensure the WiFi is enabled to perform the test.
	if err := wifiSvc.SetWifiEnabled(ctx, true); err != nil {
		s.Fatal("Failed to set WiFi enable: ", err)
	}
	// Leaving the test with WiFi enabled since WiFi being enabled is the default "good" state for group:wificell.
	defer wifiSvc.SetWifiEnabled(ctx, true)

	for _, test := range []passphraseTestCase{
		{
			name:             "incorrect_passphrase_with_8_characters",
			passphraseLength: 8,
			genPassphrase:    wpa.GenPassphrase,
			// For an incorrect but valid passphrase, expecting a notification that alerts "Bad password" will appear.
			verifications: expectIncorrectPassphraseNotification,
		}, {
			name:             "incorrect_passphrase_with_26_characters",
			passphraseLength: 26,
			genPassphrase:    wpa.GenPassphrase,
			// For an incorrect but valid passphrase, expecting a notification that alerts "Bad password" will appear.
			verifications: expectIncorrectPassphraseNotification,
		}, {
			name:             "invalid_passphrase_with_0_characters",
			passphraseLength: 0,
			genPassphrase:    wpa.GenInvalidPassphrase,
			// "Join WiFi Network" dialog only accepts passphrase more than 4 characters.
			// Passphrase has length of 0 character should be forbidden by "Join WiFi Network" dialog.
			verifications: expectConnectButtonDisabled,
		}, {
			name:             "invalid_passphrase_with_1_characters",
			passphraseLength: 1,
			genPassphrase:    wpa.GenInvalidPassphrase,
			// "Join WiFi Network" dialog only accepts passphrase more than 4 characters.
			// Passphrase has length of 1 character should be forbidden by "Join WiFi Network" dialog.
			verifications: expectConnectButtonDisabled,
		}, {
			name:             "invalid_passphrase_with_7_characters",
			passphraseLength: 7,
			genPassphrase:    wpa.GenInvalidPassphrase,
			// An invalid passphrase should be notified with a "bad password" prompt by "Join WiFi Network" dialog.
			verifications: expectBadPasswordPrompted,
		}, {
			name:             "invalid_passphrase_with_64_characters",
			passphraseLength: 64,
			genPassphrase:    wpa.GenInvalidPassphrase,
			// An invalid passphrase should be notified with a "bad password" prompt by "Join WiFi Network" dialog.
			verifications: expectBadPasswordPrompted,
		}, {
			name:          "correct_passphrase",
			genPassphrase: func(int) (string, error) { return correctPassphrase, nil },
			// The network should be connected when the correct passphrase is entered.
			verifications: expectConnected,
			// Verifying the "Disconnect" button appears on the network detail page and can disconnect to the network by clicking it.
			extraVerification: verifyDisconnectButton,
		},
	} {
		s.Run(ctx, test.name, func(ctx context.Context, s *testing.State) {
			testPassphrase, err := test.genPassphrase(test.passphraseLength)
			if err != nil {
				s.Fatal("Failed to generate test passphrase: ", err)
			}

			s.Logf("Passphrase to be tested: %q", testPassphrase)

			cleanup, err := wifiutil.JoinWifiFromQuickSettings(ctx, rpcClient.Conn, wifiutil.SecurityOptionWpa, ap.Config().SSID, testPassphrase)
			defer cleanup(cleanupCtx)
			defer wifiutil.DumpUITreeWithScreenshotToFile(cleanupCtx, rpcClient.Conn, s.HasError, test.name)
			if err != nil {
				s.Fatal("Failed to join WiFi: ", err)
			}

			if err := test.verifications(ctx, rpcClient.Conn, wifiSvc, ap.Config().SSID); err != nil {
				s.Fatal("Failed to verify that UI prompt correctly: ", err)
			}
			if test.extraVerification != nil {
				if err := test.extraVerification(ctx, rpcClient.Conn, wifiSvc, ap.Config().SSID); err != nil {
					s.Fatal("Failed to verify that UI prompt correctly: ", err)
				}
			}

		})
	}
}

// expectConnectButtonDisabled verifies the "Connect" button on "Join WiFi Network" dialog should be disabled.
func expectConnectButtonDisabled(ctx context.Context, conn *grpc.ClientConn, _ *wificell.WifiClient, ssid string) error {
	uiSvc := ui.NewAutomationServiceClient(conn)
	connectButton := wifiutil.ConnectButtonFinder
	if _, err := uiSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: connectButton}); err != nil {
		return errors.Wrap(err, "failed to find the connect button")
	}

	info, err := uiSvc.Info(ctx, &ui.InfoRequest{Finder: connectButton})
	if err != nil {
		return errors.Wrap(err, "failed to get node info of connect button")
	}

	if info.NodeInfo.Restriction != ui.Restriction_RESTRICTION_DISABLED {
		return errors.New("the connect button is not disabled")
	}
	return nil
}

// expectBadPasswordPrompted verifies a message "Bad password" should show on the "Join WiFi Network" dialog.
func expectBadPasswordPrompted(ctx context.Context, conn *grpc.ClientConn, _ *wificell.WifiClient, _ string) error {
	uiSvc := ui.NewAutomationServiceClient(conn)
	passwordErrorMsg := ui.Node().Name("Bad password").Role(ui.Role_ROLE_STATIC_TEXT).Ancestor(wifiutil.JoinWiFiNetworkDialogFinder).Finder()
	if _, err := uiSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: passwordErrorMsg}); err != nil {
		return errors.Wrap(err, "failed to find the expected error message")
	}
	return nil
}

// expectIncorrectPassphraseNotification verifies the incorrect passphrase message appears in notification center.
func expectIncorrectPassphraseNotification(ctx context.Context, conn *grpc.ClientConn, _ *wificell.WifiClient, _ string) error {
	uiSvc := ui.NewAutomationServiceClient(conn)

	if _, err := uiSvc.WaitUntilGone(ctx, &ui.WaitUntilGoneRequest{Finder: wifiutil.JoinWiFiNetworkDialogFinder}); err != nil {
		return errors.Wrap(err, "failed to wait until node gone")
	}
	badPassword := ui.Node().NameRegex(`Failed to connect to network(.*)Bad password`).Role(ui.Role_ROLE_ALERT_DIALOG).HasClass("MessagePopupView").Finder()

	if _, err := uiSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{
		Finder:  badPassword,
		Timeout: &durationpb.Duration{Seconds: int64(shillconst.DefaultTimeout.Seconds())},
	}); err != nil {
		return errors.Wrap(err, "failed to find the incorrect passphrase notification")
	}
	return nil
}

// expectConnected waits the specified network is connected.
func expectConnected(ctx context.Context, conn *grpc.ClientConn, wifiSvc *wificell.WifiClient, ssid string) error {
	return wifiSvc.WaitForConnected(ctx, ssid, true)
}

// verifyDisconnectButton verifies that the "Disconnect" button on the network detail page triggers disconnect to happen
// by examine the shill property and UI result, the "Connect" button should appear when the network is disconnected.
func verifyDisconnectButton(ctx context.Context, conn *grpc.ClientConn, wifiSvc *wificell.WifiClient, ssid string) (retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	settingsSvc := ossettings.NewOsSettingsServiceClient(conn)
	if _, err := settingsSvc.OpenNetworkDetailPage(ctx, openNetworkDetailPageRequest(ssid)); err != nil {
		return errors.Wrap(err, "failed to open network page")
	}
	defer settingsSvc.Close(cleanupCtx, &emptypb.Empty{})
	defer wifiutil.DumpUITreeWithScreenshotToFile(cleanupCtx, conn, func() bool { return retErr != nil }, "test_correct_passphrase")

	testing.ContextLog(ctx, "Verifying click the 'Disconnect' button triggers disconnect to happen")
	uiSvc := ui.NewAutomationServiceClient(conn)
	disconnectBtn := ui.Node().Name("Disconnect").Role(ui.Role_ROLE_BUTTON).Finder()
	if _, err := uiSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: disconnectBtn}); err != nil {
		return errors.Wrap(err, "failed to find the disconnect button")
	}
	if err := wifiSvc.WaitForConnected(ctx, ssid, false /* connected */); err != nil {
		return errors.Wrap(err, "failed to disconnect network")
	}

	testing.ContextLog(ctx, "Verifying the 'Connect' option reappear for a disconnected network")
	connectBtn := ui.Node().Name("Connect").Role(ui.Role_ROLE_BUTTON).Finder()
	if _, err := uiSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: connectBtn}); err != nil {
		return errors.Wrap(err, "failed to find the disconnect button")
	}
	return nil
}

func openNetworkDetailPageRequest(ssid string) *ossettings.OpenNetworkDetailPageRequest {
	return &ossettings.OpenNetworkDetailPageRequest{
		NetworkName: ssid,
		NetworkType: ossettings.OpenNetworkDetailPageRequest_WIFI,
	}
}
