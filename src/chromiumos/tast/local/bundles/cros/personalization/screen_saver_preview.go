// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package personalization

import (
	"context"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/ambient"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/personalization"
	"chromiumos/tast/testing"
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
		Timeout:      3 * time.Minute,
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
	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)

	if err := ambient.OpenAmbientSubpage(ctx, ui); err != nil {
		s.Fatal("Failed to open Ambient Subpage: ", err)
	}

	if err := ambient.EnableAmbientMode(ctx, ui); err != nil {
		s.Fatal("Failed to enable ambient mode: ", err)
	}

	previewButton := nodewith.Role(role.Button).HasClass("previewButton")
	if err := ui.LeftClick(previewButton)(ctx); err != nil {
		s.Fatal("Failed to click the preview button: ", err)
	}

	// Preview button text changes to "Downloading" when it gets disabled to load screen saver resources.
	previewButtonDisabled := nodewith.Role(role.Button).HasClass("previewButtonDisabled")
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
