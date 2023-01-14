// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wmp

import (
	"context"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/bundles/cros/wmp/wmputils"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CaptureModeDemoToolsEntryPoint,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that we can enable the demo tools feature from the capture mode settings menu",
		Contacts: []string{
			"chromeos-wm-corexp@google.com",
			"chromeos-sw-engprod@google.com",
			"michelefan@chromium.org",
		},
		// ChromeOS > Software > ScreenCapture
		BugComponent: "b:265343954",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
	})
}

func CaptureModeDemoToolsEntryPoint(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	// Start chrome and enable the demo tools feature.
	cr, err := chrome.New(ctx, chrome.EnableFeatures("CaptureModeDemoTools"))
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(ctx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	// Enter screen capture mode.
	if err := wmputils.EnsureCaptureModeActivated(tconn, true)(ctx); err != nil {
		s.Fatal("Failed to enable recording: ", err)
	}

	// Ensure case exit screen capture mode.
	defer wmputils.EnsureCaptureModeActivated(tconn, false)(cleanupCtx)

	// TODO(b/266149105): Avoid using UI text to identify the button.
	var (
		captureModeSettingsButton = nodewith.HasClass("IconButton").Name("Settings")
		demoToolsToggleButton     = nodewith.HasClass("ToggleButton").Name("Show clicks and keys")
		captureSettingsWiget      = nodewith.HasClass("CaptureModeSettingsWidget")
	)

	ac := uiauto.New(tconn)
	if err := uiauto.Combine(
		"Enable demo tools from the settings menu",
		ac.DoDefault(captureModeSettingsButton),
		ac.WaitUntilExists(captureSettingsWiget),
		// Click on the demo tools toggle button to enable the feature.
		// Use `LeftClick` is used here as `DoDefault` does not toggle
		// the toggle button. Since there is no changes in the UI tree,
		// we don't need to wait for an UI 1update in this case.
		ac.LeftClick(demoToolsToggleButton),
		// Click on the demo tools toggle button again to disable the feature.
		ac.LeftClick(demoToolsToggleButton),
	)(ctx); err != nil {
		s.Fatal("Failed to enable the demo tools feature the settings menu: ", err)
	}
}
