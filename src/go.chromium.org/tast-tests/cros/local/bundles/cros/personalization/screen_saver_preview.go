// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package personalization

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/ambient"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/personalization"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ScreenSaverPreview,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test setting previewing screen saver in the personalization hub app",
		Contacts: []string{
			"assistive-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"safarli@google.com",
		},
		// ChromeOS > Software > Personalization
		BugComponent: "b:1006527",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      6 * time.Minute,
		Fixture:      "personalizationWithScreenSaverPreviewClamshell",
	})
}

func ScreenSaverPreview(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*chrome.Chrome)

	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// The test has a dependency of network speed, so we give uiauto.Context ample
	// time to wait for nodes to load.
	ui := uiauto.New(tconn).WithTimeout(60 * time.Second)

	if err := ambient.OpenAmbientSubpage(ctx, ui); err != nil {
		s.Fatal("Failed to open Ambient Subpage: ", err)
	}

	if err := ambient.EnableAmbientMode(ctx, ui); err != nil {
		s.Fatal("Failed to enable ambient mode: ", err)
	}

	previewButton := nodewith.Role(role.Button).HasClass("preview-button")
	if err := ui.LeftClick(previewButton)(ctx); err != nil {
		s.Fatal("Failed to click the preview button: ", err)
	}

	// Preview button text changes to "Downloading" when it gets disabled to load screen saver resources.
	previewButtonDisabled := nodewith.HasClass("preview-button-disabled")
	if err := ui.WaitUntilExists(previewButtonDisabled)(ctx); err != nil {
		s.Fatal("Failed to show 'Downloading' message: ", err)
	}

	if err := ui.WaitUntilExists(
		nodewith.ClassName("InSessionAmbientModeContainer").Role(role.Window),
	)(ctx); err != nil {
		s.Fatal("Failed to start ambient mode: ", err)
	}

	if err := ambient.CloseScreenSaverPreview(ctx, tconn, ui); err != nil {
		s.Fatal("Failed to close screeen saver preview: ", err)
	}

	if err := personalization.ClosePersonalizationHub(ui)(ctx); err != nil {
		s.Fatal("Failed to close Personalization Hub: ", err)
	}
}
