// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/browser/browserui"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vkb"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OnscreenKeyboard,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Enable On-screen keyboard",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291", // ChromeOS > External > Intel
		Attr:         []string{"group:intel-nda"},
		SoftwareDeps: []string{"inputs_deps", "chrome"},
		Fixture:      "chromeLoggedIn",
	})
}

func OnscreenKeyboard(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	const onScreenKeyboard = "Keyboard and text input On-screen keyboard, dictation, Switch Access, and more"
	settings, err := ossettings.LaunchAtPage(ctx, tconn, nodewith.Name(onScreenKeyboard).Role(role.Link))
	if err != nil {
		s.Fatal("Failed to launch os-settings on-screen keyboard page: ", err)
	}
	defer settings.Close(ctx)

	cui := uiauto.New(tconn)
	onscreenButton := nodewith.Name("On-screen keyboard").Role(role.ToggleButton)
	if err := cui.LeftClick(onscreenButton)(ctx); err != nil {
		s.Fatal("Failed to find and click on On-screen keyboard: ", err)
	}

	onBoardKeyboard := nodewith.Name("On-screen keyboard").Role(role.Image)
	if err := cui.WaitUntilExists(onBoardKeyboard)(ctx); err != nil {
		s.Fatal("Failed to enable on board keyboard: ", err)
	}

	conn, err := cr.NewConn(ctx, "")
	if err != nil {
		s.Fatal("Failed to connect to chrome: ", err)
	}
	defer conn.Close()
	defer conn.CloseTarget(cleanupCtx)

	ui := uiauto.New(tconn)
	vkbCtx := vkb.NewContext(cr, tconn)

	if err := uiauto.Combine("Verify on-screen keyboard voice input and keys",
		ui.LeftClick(browserui.AddressBarFinder),
		vkbCtx.SwitchToVoiceInput(),
	)(ctx); err != nil {
		s.Fatal("Failed to verify on-screen keyboard voice input and keys: ", err)
	}
}
