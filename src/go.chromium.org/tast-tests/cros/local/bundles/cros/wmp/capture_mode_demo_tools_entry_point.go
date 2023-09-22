// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wmp

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/wmp/wmputils"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
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
		BugComponent: "b:1253115",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-773f2ac5-bc20-4c79-ad65-70fc5c8ba0c2",
		}},
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

	// Enter screen capture mode.
	if err := wmputils.EnsureCaptureModeActivated(tconn, true)(ctx); err != nil {
		s.Fatal("Failed to enable recording: ", err)
	}

	// Ensure case exit screen capture mode.
	defer wmputils.EnsureCaptureModeActivated(tconn, false)(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_dump")

	// TODO(b/266149105): Avoid using UI text to identify the button.
	var (
		captureModeSettingsButton = nodewith.HasClass("IconButton").Name("Settings")
		demoToolsToggleButton     = nodewith.HasClass("Switch").Name("Show clicks and keys")
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
