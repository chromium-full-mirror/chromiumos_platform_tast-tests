// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package clipboardhistory

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/browser/browserui"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/clipboardhistory"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type clipboardResource struct {
	ui    *uiauto.Context
	kb    *input.KeyboardEventWriter
	cr    *chrome.Chrome
	tconn *chrome.TestConn
	text  string
}

type clipboardHistoryTestParam struct {
	// source indicates the source of the pasted clipboard history data.
	source clipboardhistory.PasteSource

	// tabletMode indicates whether the test should run under the tablet mode.
	tabletMode bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ContextMenuClipboard,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies the clipboard option in the context menu is working properly within several apps by left-clicking an option",
		BugComponent: "b:1268414", // ChromeOS > Software > System UI Surfaces > EnhancedClipboard
		Contacts: []string{
			"multipaste-eng@google.com",
			"cros-system-ui-eng@google.com",
			"chromeos-consumer-engprod@google.com",
			"newcomer@google.com",
		},
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-8735ae91-8339-4836-9771-f11731dcaa7f",
		}},
		Params: []testing.Param{{
			Name: "ash",
			Val:  clipboardHistoryTestParam{clipboardhistory.ClipboardHistoryMenuFromContextMenu, false /*tabletMode=*/},
		}, {
			Name: "ash_from_submenu",
			Val:  clipboardHistoryTestParam{clipboardhistory.ClipboardHistorySubmenu, false /*tabletMode=*/},
		}, {
			Name: "ash_from_submenu_standalone_menu",
			Val:  clipboardHistoryTestParam{clipboardhistory.ClipboardHistoryMenuFromContextMenuSubmenu, false /*tabletMode=*/},
		}, {
			Name: "ash_tablet",
			Val:  clipboardHistoryTestParam{clipboardhistory.ClipboardHistoryMenuFromContextMenu, true /*tabletMode=*/},
		}, {
			Name: "ash_from_submenu_tablet",
			Val:  clipboardHistoryTestParam{clipboardhistory.ClipboardHistorySubmenu, true /*tabletMode=*/},
		}, {
			Name: "ash_from_submenu_standalone_menu_tablet",
			Val:  clipboardHistoryTestParam{clipboardhistory.ClipboardHistoryMenuFromContextMenuSubmenu, true /*tabletMode=*/},
		}},
	})
}

// ContextMenuClipboard verifies that it is possible to open clipboard history
// via various surfaces' context menus.
func ContextMenuClipboard(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	testParam := s.Param().(clipboardHistoryTestParam)

	// Enable the clipboard history refresh feature if the paste source requires
	// the clipboard history submenu.
	var option chrome.Option
	var features = []string{"ClipboardHistoryRefresh"}
	if testParam.source == clipboardhistory.ClipboardHistorySubmenu ||
		testParam.source == clipboardhistory.ClipboardHistoryMenuFromContextMenuSubmenu {
		option = chrome.EnableFeatures(features...)
	} else {
		option = chrome.DisableFeatures(features...)
	}

	cr, err := chrome.New(ctx, option)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, testParam.tabletMode)
	if err != nil {
		s.Fatalf("Failed to set tablet mode to be %v: %v", testParam.tabletMode, err)
	}
	defer cleanup(ctx)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctx)

	res := &clipboardResource{
		ui:    uiauto.New(tconn),
		kb:    kb,
		cr:    cr,
		tconn: tconn,
		text:  "abc",
	}

	if err := ash.SetClipboard(cleanupCtx, res.tconn, res.text); err != nil {
		s.Fatalf("Failed to add %q to clipboard history: %v", res.text, err)
	}

	s.Run(cleanupCtx, "verify Chrome", func(ctx context.Context, s *testing.State) {
		verifyChrome(ctx, s, res, testParam.source)
	})
	s.Run(cleanupCtx, "verify Settings", func(ctx context.Context, s *testing.State) {
		verifySettings(ctx, s, res, testParam.source)
	})
	s.Run(cleanupCtx, "verify Launcher", func(ctx context.Context, s *testing.State) {
		verifyLauncher(ctx, res.tconn, s, testParam.tabletMode, res, testParam.source)
	})
}

func verifyChrome(ctx context.Context, s *testing.State, res *clipboardResource, source clipboardhistory.PasteSource) {
	conn, err := res.cr.NewConn(ctx, "")
	if err != nil {
		s.Fatal("Failed to connect to Chrome: ", err)
	}
	defer conn.Close()
	defer conn.CloseTarget(ctx)
	defer faillog.DumpUITreeWithScreenshotOnError(
		ctx, s.OutDir(), s.HasError, res.cr, fmt.Sprintf("%s_dump", s.TestName()))

	if err := clipboardhistory.PasteAndVerify(res.tconn, res.ui, res.kb,
		browserui.AddressBarFinder, source, res.text, clipboardhistory.Click)(ctx); err != nil {
		s.Fatal("Failed to paste to Chrome and verify: ", err)
	}
}

func verifySettings(ctx context.Context, s *testing.State, res *clipboardResource, source clipboardhistory.PasteSource) {
	settings, err := ossettings.Launch(ctx, res.tconn)
	if err != nil {
		s.Fatal("Failed to launch Settings: ", err)
	}
	defer settings.Close(ctx)
	defer faillog.DumpUITreeWithScreenshotOnError(
		ctx, s.OutDir(), s.HasError, res.cr, fmt.Sprintf("%s_dump", s.TestName()))

	if err := clipboardhistory.PasteAndVerify(res.tconn, res.ui, res.kb,
		ossettings.SearchBoxFinder, source, res.text, clipboardhistory.Click)(ctx); err != nil {
		s.Fatal("Failed to paste to Settings and verify: ", err)
	}
}

func verifyLauncher(ctx context.Context, tconn *chrome.TestConn, s *testing.State, tabletMode bool,
	res *clipboardResource, source clipboardhistory.PasteSource) {
	if err := launcher.ShowLauncher(res.tconn, !tabletMode)(ctx); err != nil {
		s.Fatal("Failed to connect to open Launcher: ", err)
	}
	defer launcher.HideLauncher(res.tconn, !tabletMode)(ctx)
	defer faillog.DumpUITreeWithScreenshotOnError(
		ctx, s.OutDir(), s.HasError, res.cr, fmt.Sprintf("%s_dump", s.TestName()))

	search := nodewith.HasClass("SearchBoxView")
	searchbox := nodewith.HasClass("Textfield").Role(role.TextField).Ancestor(search)
	if err := clipboardhistory.PasteAndVerify(res.tconn, res.ui, res.kb, searchbox, source, res.text, clipboardhistory.Click)(ctx); err != nil {
		s.Fatal("Failed to paste to Launcher and verify: ", err)
	}
}
