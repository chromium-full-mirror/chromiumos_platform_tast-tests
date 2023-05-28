// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package clipboardhistory

import (
	"context"
	"fmt"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/browser/browserui"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/clipboardhistory"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/testing"
)

type clipboardResource struct {
	ui    *uiauto.Context
	kb    *input.KeyboardEventWriter
	cr    *chrome.Chrome
	bt    browser.Type
	tconn *chrome.TestConn
	text  string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ContextMenuClipboard,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verifies the clipboard option in the context menu is working properly within several apps by left-clicking an option",
		BugComponent: "b:1268414", // ChromeOS > Software > System UI Surfaces > EnhancedClipboard
		Contacts: []string{
			"multipaste-eng@google.com",
			"cros-system-ui-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"ckincaid@google.com",
		},
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Name:    "ash",
			Val:     browser.TypeAsh,
			Fixture: "chromeLoggedIn",
		}, {
			Name:              "lacros",
			Val:               browser.TypeLacros,
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           "lacros",
		}},
	})
}

// ContextMenuClipboard verifies that it is possible to open clipboard history
// via various surfaces' context menus.
func ContextMenuClipboard(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctx)

	res := &clipboardResource{
		ui:    uiauto.New(tconn),
		kb:    kb,
		cr:    cr,
		bt:    s.Param().(browser.Type),
		tconn: tconn,
		text:  "abc",
	}

	if err := ash.SetClipboard(ctx, res.tconn, res.text); err != nil {
		s.Fatalf("Failed to add %q to clipboard history: %v", res.text, err)
	}

	s.Run(ctx, "verify Chrome", func(ctx context.Context, s *testing.State) {
		verifyChrome(ctx, s, res)
	})
	s.Run(ctx, "verify Settings", func(ctx context.Context, s *testing.State) {
		verifySettings(ctx, s, res)
	})
	s.Run(ctx, "verify Launcher", func(ctx context.Context, s *testing.State) {
		verifyLauncher(ctx, s, res)
	})
}

func verifyChrome(ctx context.Context, s *testing.State, res *clipboardResource) {
	br, closeBrowser, err := browserfixt.SetUp(ctx, res.cr, res.bt)
	if err != nil {
		s.Fatal("Failed to open the browser: ", err)
	}
	defer closeBrowser(ctx)

	conn, err := br.NewConn(ctx, "")
	if err != nil {
		s.Fatal("Failed to connect to Chrome: ", err)
	}
	defer conn.Close()
	defer conn.CloseTarget(ctx)
	defer faillog.DumpUITreeWithScreenshotOnError(
		ctx, s.OutDir(), s.HasError, res.cr, fmt.Sprintf("%s_dump", s.TestName()))

	if err := clipboardhistory.PasteAndVerify(res.tconn, res.ui, res.kb, browserui.AddressBarFinder, true /*useContextMenu*/, res.text, clipboardhistory.Click)(ctx); err != nil {
		s.Fatal("Failed to paste to Chrome and verify: ", err)
	}
}

func verifySettings(ctx context.Context, s *testing.State, res *clipboardResource) {
	settings, err := ossettings.Launch(ctx, res.tconn)
	if err != nil {
		s.Fatal("Failed to launch Settings: ", err)
	}
	defer settings.Close(ctx)
	defer faillog.DumpUITreeWithScreenshotOnError(
		ctx, s.OutDir(), s.HasError, res.cr, fmt.Sprintf("%s_dump", s.TestName()))

	if err := clipboardhistory.PasteAndVerify(res.tconn, res.ui, res.kb, ossettings.SearchBoxFinder, true /*useContextMenu*/, res.text, clipboardhistory.Click)(ctx); err != nil {
		s.Fatal("Failed to paste to Settings and verify: ", err)
	}
}

func verifyLauncher(ctx context.Context, s *testing.State, res *clipboardResource) {
	if err := launcher.OpenBubbleLauncher(res.tconn)(ctx); err != nil {
		s.Fatal("Failed to connect to open Launcher: ", err)
	}
	defer launcher.CloseBubbleLauncher(res.tconn)(ctx)
	defer faillog.DumpUITreeWithScreenshotOnError(
		ctx, s.OutDir(), s.HasError, res.cr, fmt.Sprintf("%s_dump", s.TestName()))

	search := nodewith.HasClass("SearchBoxView")
	searchbox := nodewith.HasClass("Textfield").Role(role.TextField).Ancestor(search)
	if err := clipboardhistory.PasteAndVerify(res.tconn, res.ui, res.kb, searchbox, true /*useContextMenu*/, res.text, clipboardhistory.Click)(ctx); err != nil {
		s.Fatal("Failed to paste to Launcher and verify: ", err)
	}
}
