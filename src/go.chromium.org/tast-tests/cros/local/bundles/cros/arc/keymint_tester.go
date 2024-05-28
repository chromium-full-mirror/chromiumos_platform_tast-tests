// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	uiCommon "go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/apputil"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	testerAppName     = "KeyMintTester"
	testerPackageName = "com.google.asap.keyminttester"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         KeymintTester,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "A functional test using a KeyMintTester app",
		Contacts:     []string{"arc-commercial@google.com", "yaohuali@google.com"},
		// ChromeOS > Software > ARC++ > Commercial > Tast Tests
		BugComponent: "b:1487630",
		Attr:         []string{"group:mainline", "informational"},
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
		Fixture:      "arcBooted",
		// TODO(b/301347001): Enable this test for ARC T+.
		SoftwareDeps: []string{"android_vm_t", "chrome"},
		VarDeps:      []string{uiCommon.GaiaPoolDefaultVarName},
		Params: []testing.Param{{
			ExtraData: []string{
				"KeyMintTester.apk",
			},
			Val: "KeyMintTester.apk",
		}},
	})
}

func KeymintTester(ctx context.Context, s *testing.State) {
	const (
		createKeysButtonText = "CREATE\nKEYS"
		checkKeysButtonText  = "CHECK\nKEYS"

		authKeyCreatedText = "Created AuthKey"
		keysNotCreatedText = "Error loading data - did you create keys yet?"
		keysCreatedText    = "Created 50 of 50 keys"
		keysCheckedText    = "Checked 50 of 50 keys"
	)

	a := s.FixtValue().(*arc.PreData).ARC
	cr := s.FixtValue().(*arc.PreData).Chrome
	d := s.FixtValue().(*arc.PreData).UIDevice
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard controller: ", err)
	}
	defer kb.Close(ctx)

	testerApkName := s.Param().(string)
	if err := a.Install(ctx, s.DataPath(testerApkName)); err != nil {
		s.Fatal("Failed to install KeyMintTester APK: ", err)
	}

	app, err := apputil.NewApp(ctx, kb, tconn, a, d, testerAppName, testerPackageName)
	if err != nil {
		s.Fatal("Failed to create the instance of KeyMintTester app: ", err)
	}

	if _, err := app.Launch(ctx); err != nil {
		s.Fatal("Failed to launch KeyMintTester app: ", err)
	}

	errText := d.Object(ui.ID("com.google.asap.keyminttester:id/messages"))
	if err := errText.WaitForExists(ctx, 5*time.Second); err != nil {
		s.Fatal("Failed to find textview object: ", err)
	}
	if err := waitForText(ctx, errText, authKeyCreatedText, 5*time.Second); err != nil {
		s.Fatal("Failed to find expected text prior to creating keys: ", err)
	}
	if err := waitForText(ctx, errText, keysNotCreatedText, 5*time.Second); err != nil {
		s.Fatal("Failed to find expected text prior to creating keys: ", err)
	}

	createKeysButton := d.Object(ui.ClassName("android.widget.Button"), ui.TextMatches(createKeysButtonText))
	if err := createKeysButton.WaitForExists(ctx, 5*time.Second); err != nil {
		s.Fatal("Failed to find CREATE KEYS button: ", err)
	}
	if err := createKeysButton.Click(ctx); err != nil {
		s.Fatal("Failed to click CREATE KEYS button: ", err)
	}
	if err := waitForText(ctx, errText, keysCreatedText, 10*time.Second); err != nil {
		s.Fatal("Failed to find expected text after creating keys: ", err)
	}

	checkKeysButton := d.Object(ui.ClassName("android.widget.Button"), ui.TextMatches(checkKeysButtonText))
	if err := checkKeysButton.WaitForExists(ctx, 5*time.Second); err != nil {
		s.Fatal("Failed to find CHECK KEYS button: ", err)
	}
	if err := checkKeysButton.Click(ctx); err != nil {
		s.Fatal("Failed to click CHECK KEYS button: ", err)
	}
	if err := waitForText(ctx, errText, keysCheckedText, 5*time.Second); err != nil {
		s.Fatal("Failed to find expected text after checking keys: ", err)
	}
}

func waitForText(ctx context.Context, obj *ui.Object, expectedText string, timeout time.Duration) error {
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
