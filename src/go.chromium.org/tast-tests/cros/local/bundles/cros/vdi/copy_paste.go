// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package vdi

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/uidetection"
	vdiApps "go.chromium.org/tast-tests/cros/local/vdi/apps"
	"go.chromium.org/tast-tests/cros/local/vdi/fixtures"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CopyPaste,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Copy and paste text between Notepad VDI application and native text editor application",
		Contacts: []string{
			"cros-engprod-muc@google.com",
			"hendrich@google.com", // Test author
		},
		BugComponent: "b:1170223", // ChromeOS > Software > Commercial (Enterprise) > EngProd
		Attr:         []string{},
		SoftwareDeps: []string{"chrome"},
		Timeout:      5 * time.Minute,
		SearchFlags: []*testing.StringPair{{
			Key: "feature_id",
			// copy data from Microsoft Word (we use Notepad as equivalent here) file in VDI: COM_VDI_CUJ5_TASK1_WF1
			Value: "screenplay-5a9af21d-c74d-4a76-959b-f761cb2fbe2d",
		}, {
			Key: "feature_id",
			// paste copied data into Google Document: COM_VDI_CUJ5_TASK2_WF1
			Value: "screenplay-5de4a98c-1472-4ff4-a4bb-7c576543437e",
		}},
		Requirements: []string{"screenplay-bbe00399-7b52-49ec-8402-9440c89f412c", "screenplay-ff8ef471-f3ce-4252-bee1-e7afcacd242a"},
		Params: []testing.Param{
			{
				Name:      "citrix",
				Fixture:   fixture.CitrixLaunched,
				// ExtraAttr: []string{"group:vdi_limited"}, TODO(b/263381075) fix failing test
			},
			{
				Name:              "lacros_citrix",
				Fixture:           fixture.LacrosCitrixLaunched,
				// ExtraAttr:         []string{"group:vdi_limited"}, TODO(b/263381075) fix failing test
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name:      "mgs_citrix",
				Fixture:   fixture.MgsCitrixLaunched,
				// ExtraAttr: []string{"group:vdi_limited"}, TODO(b/263381075) fix failing test
			},
			{
				Name:              "mgs_lacros_citrix",
				Fixture:           fixture.MgsLacrosCitrixLaunched,
				// ExtraAttr:         []string{"group:vdi_limited"}, TODO(b/263381075) fix failing test
				ExtraSoftwareDeps: []string{"lacros"},
			},
			// TODO(b/263381075): VMWare doesn't have as simple apps as notepad.
		},
	})
}

func CopyPaste(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	vdi := s.FixtValue().(fixtures.HasVDIConnector).VDIConnector()
	uidetector := s.FixtValue().(fixtures.HasUIDetector).UIDetector()

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Get keyboard.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(ctx)

	if err := tconn.WaitForExpr(ctx, "chrome.clipboard"); err != nil {
		s.Fatal("Failed to wait for chrome.clipboard API to become available: ", err)
	}

	defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree")

	var remoteTextAppName = "Notepad"
	var remoteTextToLookForWhenLaunched = "Untitled - Notepad"

	var nativeAppURL = "chrome-extension://" + apps.Text.ID + "/index.html"

	const text1 = "Test123"
	var text2 = strings.Repeat(text1, 2)
	var text4 = strings.Repeat(text1, 4)

	// Type text in remote app, select it and copy it.
	doOrFail(launchRemoteTextApp(ctx, tconn, uidetector, vdi, remoteTextAppName, remoteTextToLookForWhenLaunched), s, "failed to launch remote text app")
	doOrFail(pressKeys(ctx, kb, "Ctrl+A"), s, "failed to select all text in remote app") // TODO(kamilszarek@google.com) can be removed once VDI sessions are properly cleaned-up after test
	doOrFail(typeTextAndCheck(ctx, kb, uidetector, text1), s, "failed to type text in remote app")
	doOrFail(pressKeys(ctx, kb, "Ctrl+A"), s, "failed to select all text in remote app")
	doOrFail(pressKeys(ctx, kb, "Ctrl+C"), s, "failed to copy text in remote app")
	doOrFail(checkClipboardContent(ctx, tconn, text1), s, "failed to check clipboard content from remote app")

	// Paste text in native app twice and check.
	doOrFail(launchNativeTextApp(ctx, tconn, cr, apps.Text.ID, nativeAppURL), s, "failed to launch native text app")
	doOrFail(pressKeys(ctx, kb, "Ctrl+V"), s, "failed to paste text into native app #1")
	doOrFail(pressKeys(ctx, kb, "Ctrl+V"), s, "failed to paste text into native app #2")
	doOrFail(waitForText(ctx, uidetector, text2), s, "failed to find 2x pasted text in native app")

	// Copy from native app.
	doOrFail(pressKeys(ctx, kb, "Ctrl+A"), s, "failed to select all text in native app")
	doOrFail(pressKeys(ctx, kb, "Ctrl+C"), s, "failed to copy text in native app")
	doOrFail(checkClipboardContent(ctx, tconn, text2), s, "failed to check clipboard content from native app")

	// Paste text in remote app twice and check.
	doOrFail(focusWindow(ctx, tconn, remoteTextAppName), s, "failed to focus remote app window")
	doOrFail(pressKeys(ctx, kb, "Ctrl+A"), s, "failed to select all text in remote app")
	doOrFail(pressKeys(ctx, kb, "Ctrl+V"), s, "failed to paste text into remote app #1")
	doOrFail(pressKeys(ctx, kb, "Ctrl+V"), s, "failed to paste text into remote app #2")
	doOrFail(waitForText(ctx, uidetector, text4), s, "failed to find 4x pasted text in remote app")
}

func doOrFail(err error, s *testing.State, contextMessage string) {
	if err != nil {
		s.Fatalf("%s: %v", contextMessage, err)
	}
}

func launchRemoteTextApp(ctx context.Context, tconn *chrome.TestConn, uidetector *uidetection.Context, vdi vdiApps.VDIInt, appName, textToLookForWhenLaunched string) error {
	testing.ContextLogf(ctx, "Launching remote text app: %s", appName)

	isOpened := func(ctx context.Context) error {
		// Wait for actual application window to open.
		if _, err := ash.WaitForAnyWindowWithTitle(ctx, tconn, appName); err != nil {
			return errors.Wrap(err, "failed to find remote app window")
		}

		return waitForText(ctx, uidetector, textToLookForWhenLaunched)
	}

	if err := vdi.SearchAndOpenApplication(ctx, appName, isOpened)(ctx); err != nil {
		return errors.Wrap(err, "failed to open remote app")
	}

	// The VDI app window can be in a weird focus state, where the window on ChromeOS has focus, but the window on Windows (inside the VDI session) does not. Clicking somewhere inside the window will properly set focus for both.
	var textBlocks = strings.Split(textToLookForWhenLaunched, " ")
	if err := uidetector.LeftClick(uidetection.TextBlock(textBlocks))(ctx); err != nil {
		return errors.Wrap(err, "failed to click on launched window")
	}

	return nil
}

func launchNativeTextApp(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome, appID, appURL string) error {
	testing.ContextLogf(ctx, "Launching native text app: %s", appID)

	if err := ash.WaitForChromeAppInstalled(ctx, tconn, appID, 2*time.Minute); err != nil {
		return errors.Wrap(err, "failed to wait for native app to be installed")
	}

	if err := apps.Launch(ctx, tconn, appID); err != nil {
		return errors.Wrap(err, "failed to launch native app")
	}

	_, err := ash.WaitForAppWindow(ctx, tconn, appID)
	if err != nil {
		return errors.Wrap(err, "failed to wait for native app to be launched")
	}

	appConn, err := cr.NewConnForTarget(ctx, chrome.MatchTargetURL(appURL))
	if err != nil {
		return errors.Wrap(err, "failed to get connection to native app")
	}
	if err := appConn.WaitForExpr(ctx, `document.activeElement.type === "textarea"`); err != nil {
		return errors.Wrap(err, "failed to wait for native app to finish loading")
	}

	return nil
}

func focusWindow(ctx context.Context, tconn *chrome.TestConn, windowTitle string) error {
	testing.ContextLogf(ctx, "Focusing window: %s", windowTitle)

	matchTitle := func(w *ash.Window) bool {
		return strings.Contains(w.Title, windowTitle)
	}

	w, err := ash.FindOnlyWindow(ctx, tconn, matchTitle)
	if err != nil {
		return errors.Wrap(err, "could not find window with matching title")
	}

	if err := w.ActivateWindow(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to activate window")
	}

	return nil
}

func waitForText(ctx context.Context, uidetector *uidetection.Context, text string) error {
	testing.ContextLogf(ctx, "Checking for text block: %s", text)

	var textBlocks = strings.Split(text, " ")
	if err := uidetector.WaitUntilExists(uidetection.TextBlock(textBlocks))(ctx); err != nil {
		return errors.Wrap(err, "Did not find text block")
	}
	return nil
}

func checkClipboardContent(ctx context.Context, tconn *chrome.TestConn, expectedContent string) error {
	testing.ContextLogf(ctx, "Checking clipboard content (expected: %q)", expectedContent)

	var clipData string
	if err := tconn.Eval(ctx, `tast.promisify(chrome.autotestPrivate.getClipboardTextData)()`, &clipData); err != nil {
		return errors.Wrap(err, "failed to get clipboard content")
	}
	if clipData != expectedContent {
		return errors.New("clipboard content '" + clipData + "' does not match expected content '" + expectedContent + "'")
	}

	return nil
}

// pressKeys adds a small sleep afterwards since the key actions are delayed on the VDI side.
func pressKeys(ctx context.Context, kb *input.KeyboardEventWriter, keys string) error {
	testing.ContextLogf(ctx, "Pressing %q", keys)

	if err := uiauto.Combine("Press keys with timeout",
		kb.AccelAction(keys),
		uiauto.Sleep(time.Second),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to press keys")
	}

	return nil
}

func typeTextAndCheck(ctx context.Context, kb *input.KeyboardEventWriter, uidetector *uidetection.Context, text string) error {
	testing.ContextLogf(ctx, "Typing %q", text)

	if err := kb.Type(ctx, text); err != nil {
		return errors.Wrap(err, "failed to type text")
	}

	if err := waitForText(ctx, uidetector, text); err != nil {
		return errors.Wrap(err, "could not find typed text")
	}

	return nil
}
