// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/apputil"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         KeymintMigration,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "A key migration test",
		Contacts:     []string{"arc-commercial@google.com", "yaohuali@google.com"},
		// ChromeOS > Software > ARC++ > Commercial > Secret Management
		BugComponent: "b:1284082",
		Attr:         []string{"group:mainline", "informational"},
		Timeout:      7 * time.Minute,
		// TODO(b/301347001): Enable this test for ARC T+.
		SoftwareDeps: []string{"android_vm_t", "chrome"},
		VarDeps:      []string{"ui.gaiaPoolDefault"},
		Params: []testing.Param{{
			ExtraData: []string{
				"ecdh_and_x25519.apk",
			},
			Val: "ecdh_and_x25519.apk",
		}},
	})
}

// KeymintMigration tests the migration from ARC-T Keymaster to ARC-T KeyMint.
// The keys created with Keymaster should still be usable after switching to KeyMint.
func KeymintMigration(ctx context.Context, s *testing.State) {
	const (
		testerAppName     = "ECDH Security Test"
		testerPackageName = "com.example.ecdhtest"

		generateEcdhKeyPairButtonID      = "com.example.ecdhtest:id/generateECDHButton"
		generateEcdhKeySuccessfulMessage = "generateECDH successful, use Strongbox: false"
		ecdhKeyExchangeButtonID          = "com.example.ecdhtest:id/exchangeButton"
		ecdhKeyExchangeSuccessfulMessage = "ECDH Key Exchange Successful"

		generateX25519KeyPairButtonID      = "com.example.ecdhtest:id/generateX25519Button"
		generateX25519KeySuccessfulMessage = "generateX25519 successful, use Strongbox: false"
		x25519KeyExchangeButtonID          = "com.example.ecdhtest:id/exchangeX25519Button"
		x25519KeyExchangeSuccessfulMessage = "X25519 Key Exchange Successful"
	)

	{
		cr, err := chrome.New(ctx,
			chrome.ARCEnabled(),
			chrome.SkipForceOnlineSignInForTesting(),
			chrome.DisableFeatures("ArcSwitchToKeyMintOnT", "ArcSwitchToKeyMintOnTOverride"),
			chrome.ExtraArgs(arc.DisableSyncFlags()...),
		)
		if err != nil {
			s.Fatal("Failed to restart Chrome for testing: ", err)
		}
		defer cr.Close(ctx)

		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			s.Fatal("Failed to create test API connection: ", err)
		}
		defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

		a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
		if err != nil {
			s.Fatal("Failed to start ARC: ", err)
		}
		defer a.Close(ctx)

		d, err := a.NewUIDevice(ctx)
		if err != nil {
			s.Fatal("Failed initializing UI Automator: ", err)
		}
		defer d.Close(ctx)

		kb, err := input.Keyboard(ctx)
		if err != nil {
			s.Fatal("Failed to get keyboard controller: ", err)
		}
		defer kb.Close(ctx)

		testerApkName := s.Param().(string)
		if err := a.Install(ctx, s.DataPath(testerApkName)); err != nil {
			s.Fatal("Failed to install ecdh_and_x25519 APK: ", err)
		}

		app, err := apputil.NewApp(ctx, kb, tconn, a, d, testerAppName, testerPackageName)
		if err != nil {
			s.Fatal("Failed to create the instance of ecdh_and_x25519 app: ", err)
		}

		if _, err := app.Launch(ctx); err != nil {
			s.Fatal("Failed to launch ecdh_and_x25519 app: ", err)
		}

		// Click "Generate ECDH Key Pair" button
		if err := clickButtonAndVerifyResult(ctx, d, generateEcdhKeyPairButtonID, generateEcdhKeySuccessfulMessage); err != nil {
			s.Fatal("Failed to Generate ECDH Keypair: ", err)
		}

		// Click "Generate X25519 Key Pair" button
		if err := clickButtonAndVerifyResult(ctx, d, generateX25519KeyPairButtonID, generateX25519KeySuccessfulMessage); err != nil {
			s.Fatal("Failed to Generate X25519 Keypair: ", err)
		}

		// close
		if err := app.Close(ctx, cr, func() bool { return false }, s.OutDir()); err != nil {
			s.Fatal("Failed to close app: ", err)
		}
	}
	// User logged out, and ready to start a new session.
	{
		// The second login must be the same as the first. Here we use the same default user.
		cr, err := chrome.New(ctx,
			chrome.ARCEnabled(),
			chrome.SkipForceOnlineSignInForTesting(),
			chrome.KeepState(),
			chrome.EnableFeatures("ArcSwitchToKeyMintOnT", "ArcSwitchToKeyMintOnTOverride"),
			chrome.ExtraArgs(arc.DisableSyncFlags()...),
		)
		if err != nil {
			s.Fatal("Failed to restart Chrome for testing: ", err)
		}
		defer cr.Close(ctx)

		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			s.Fatal("Failed to create test API connection: ", err)
		}
		defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

		a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
		if err != nil {
			s.Fatal("Failed to start ARC: ", err)
		}
		defer a.Close(ctx)

		d, err := a.NewUIDevice(ctx)
		if err != nil {
			s.Fatal("Failed initializing UI Automator: ", err)
		}
		defer d.Close(ctx)

		kb, err := input.Keyboard(ctx)
		if err != nil {
			s.Fatal("Failed to get keyboard controller: ", err)
		}
		defer kb.Close(ctx)

		// Launch tester app
		app, err := apputil.NewApp(ctx, kb, tconn, a, d, testerAppName, testerPackageName)
		if err != nil {
			s.Fatal("Failed to create the instance of ecdh_and_x25519 app: ", err)
		}

		if _, err := app.Launch(ctx); err != nil {
			s.Fatal("Failed to launch ecdh_and_x25519 app: ", err)
		}

		// ECDH key exchange.
		if err := clickButtonAndVerifyResult(ctx, d, ecdhKeyExchangeButtonID, ecdhKeyExchangeSuccessfulMessage); err != nil {
			s.Fatal("Failed in ECDH key exchange: ", err)
		}

		// X25519 key exchange.
		if err := clickButtonAndVerifyResult(ctx, d, x25519KeyExchangeButtonID, x25519KeyExchangeSuccessfulMessage); err != nil {
			s.Fatal("Failed in X25519 key exchange: ", err)
		}
	}
}

func waitForTextInUI(ctx context.Context, obj *ui.Object, expectedText string, timeout time.Duration) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		text, err := obj.GetText(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get text in UI object")
		}
		if !strings.Contains(text, expectedText) {
			return errors.Errorf("failed to find expected text: %s", expectedText)
		}
		return nil
	}, &testing.PollOptions{Timeout: timeout})
}

// clickButtonAndVerifyResult clicks button with |buttonId|, and verifies that the result pop-up contains string |expectedResult|.
func clickButtonAndVerifyResult(ctx context.Context, d *ui.Device, buttonID, expectedResult string) error {
	const (
		popUpMessageID = "android:id/message"
		closePopUpID   = "android:id/button1"
	)
	// Click button to conduct operation
	targetButton := d.Object(ui.ID(buttonID))
	if err := targetButton.WaitForExists(ctx, 5*time.Second); err != nil {
		return errors.Wrapf(err, "failed to find %s button", buttonID)
	}
	if err := targetButton.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to click Generate ECDH Keypair button")
	}

	// Wait for pop-up to show success result.
	resultPopUp := d.Object(ui.ID(popUpMessageID))
	if err := resultPopUp.WaitForExists(ctx, 5*time.Second); err != nil {
		return errors.Wrap(err, "failed to see pop-up from Generate ECDH Keypair")
	}
	if err := waitForTextInUI(ctx, resultPopUp, expectedResult, 5*time.Second); err != nil {
		return errors.Wrap(err, "failed to find success text after Generate ECDH Keypair")
	}

	resultCloseButton := d.Object(ui.ID(closePopUpID))
	if err := resultCloseButton.WaitForExists(ctx, 2*time.Second); err != nil {
		return errors.Wrap(err, "failed to see CLOSE button on result pop-up from Generate ECDH Keypair")
	}
	if err := resultCloseButton.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to click CLOSE button on result pop-up from Generate ECDH Keypair")
	}

	return nil
}
