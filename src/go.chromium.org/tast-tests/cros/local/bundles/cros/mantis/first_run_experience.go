// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mantis

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/mantis/constant"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/mantis/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/mantis/util"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/galleryapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FirstRunExperience,
		Desc: "Verify FRE dialog behavior",
		Contacts: []string{
			"cros-mantis@google.com",
			"nurlitadf@google.com",
		},
		BugComponent: "b:1445284",
		Timeout:      constant.DefaultTestTimeout,
		SoftwareDeps: []string{"chrome", "chrome_internal", "dlc"},
		HardwareDeps: hwdep.D(hwdep.Model("navi")),
		Data:         []string{constant.ImageTestFileName},
		Attr:         []string{"group:mainline", "informational"},
		Fixture:      fixture.LoggedInWithUpdateEngine,
	})
}

func FirstRunExperience(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.Data).Chrome
	tconn := s.FixtValue().(fixture.Data).TestAPIConn
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	if err := util.DownloadAndOpenFileInGallery(ctx, cr, s.DataPath(constant.ImageTestFileName), constant.ImageTestFileName); err != nil {
		s.Fatal("Failed to download and open file in Gallery: ", err)
	}

	// Connect to the Gallery app HTML page,
	// where JavaScript can be executed to simulate interactions with the UI.
	crconn, err := cr.NewConnForTarget(ctx, chrome.MatchTargetURL("chrome-untrusted://media-app/app.html"))
	if err != nil {
		s.Fatal("Failed to get connection to Gallery app: ", err)
	}
	if err := crconn.Call(ctx, nil, `() => {window.localStorage.setItem('edit-with-ai-onboarding-9xVt6pQ3G', 'false');}`); err != nil {
		s.Fatal("Failed to set local storage: ", err)
	}

	ui := uiauto.New(tconn)

	// The first time the user open 'Edit with AI' panel, FRE dialog should be shown
	editWithAIButton := nodewith.Role(role.ToggleButton).Name("Edit with AI").Ancestor(galleryapp.RootFinder)
	if err := ui.DoDefault(editWithAIButton)(ctx); err != nil {
		s.Fatal("Unable to click 'Edit with AI' button: ", err)
	}
	freHeadingText := nodewith.Role(role.StaticText).NameContaining("How to edit with AI").Ancestor(galleryapp.RootFinder)
	if err := ui.WithTimeout(constant.DefaultUITimeout).WaitUntilExists(freHeadingText)(ctx); err != nil {
		s.Fatal("FRE isn't shown: ", err)
	}

	freDismissButton := nodewith.Role(role.Button).Name("Got it").Ancestor(galleryapp.RootFinder)
	if err := ui.DoDefault(freDismissButton)(ctx); err != nil {
		s.Fatal("Failed to click on FRE dismiss button: ", err)
	}

	if err := util.CloseGallery(ctx, tconn); err != nil {
		s.Fatal("Failed to close Gallery: ", err)
	}

	// Reopen Gallery app.
	if err := util.OpenGalleryFromDownload(ctx, ui, tconn, constant.ImageTestFileName); err != nil {
		s.Fatal("Failed to reopen Gallery: ", err)
	}

	// The second time the user open 'Edit with AI' panel, FRE dialog should not be shown
	if err := ui.DoDefault(editWithAIButton)(ctx); err != nil {
		s.Fatal("Unable to click 'Edit with AI' button: ", err)
	}
	if err := ui.WithTimeout(constant.DefaultUITimeout).WaitUntilExists(freHeadingText)(ctx); err == nil {
		s.Fatal("FRE should not be shown the second time")
	}
}
