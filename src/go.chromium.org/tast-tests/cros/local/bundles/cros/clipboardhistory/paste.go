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
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/browser/browserui"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/clipboardhistory"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/testing"
)

type pasteTestParams struct {
	pasteType   clipboardhistory.PasteType
	browserType browser.Type
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         Paste,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verifies different methods for pasting from clipboard history",
		BugComponent: "b:1268414", // ChromeOS > Software > System UI Surfaces > EnhancedClipboard
		Contacts: []string{
			"multipaste-eng@google.com",
			"cros-system-ui-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"ckincaid@google.com",
		},
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-8bcf8422-65f2-44c9-8a12-c1ebf9807271",
		}},
		Params: []testing.Param{{
			Name: "click_ash",
			Val: pasteTestParams{
				pasteType:   clipboardhistory.Click,
				browserType: browser.TypeAsh,
			},
			Fixture: "chromeLoggedIn",
		}, {
			Name: "click_lacros",
			Val: pasteTestParams{
				pasteType:   clipboardhistory.Click,
				browserType: browser.TypeLacros,
			},
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           "lacros",
		}, {
			Name: "enter_ash",
			Val: pasteTestParams{
				pasteType:   clipboardhistory.Enter,
				browserType: browser.TypeAsh,
			},
			Fixture: "chromeLoggedIn",
		}, {
			Name: "enter_lacros",
			Val: pasteTestParams{
				pasteType:   clipboardhistory.Enter,
				browserType: browser.TypeLacros,
			},
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           "lacros",
		}, {
			Name: "toggle_ash",
			Val: pasteTestParams{
				pasteType:   clipboardhistory.Toggle,
				browserType: browser.TypeAsh,
			},
			Fixture: "chromeLoggedIn",
		}, {
			Name: "toggle_lacros",
			Val: pasteTestParams{
				pasteType:   clipboardhistory.Toggle,
				browserType: browser.TypeLacros,
			},
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           "lacros",
		}},
	})
}

// Paste verifies clipboard history paste behavior. It is expected that users
// can paste from the clipboard history menu by clicking on a history item,
// pressing enter with a history item selected, or toggling the menu closed with
// a history item selected.
func Paste(ctx context.Context, s *testing.State) {
	// It is fine not to use a new Chrome instance as long as we always add an
	// item to clipboard history and expect it to be the menu's top item.
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	ui := uiauto.New(tconn).WithTimeout(10 * time.Second)

	params := s.Param().(pasteTestParams)

	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, params.browserType)
	if err != nil {
		s.Fatal("Failed to open the browser: ", err)
	}
	defer closeBrowser(ctx)

	conn, err := br.NewConn(ctx, "")
	if err != nil {
		s.Fatal("Failed to connect to chrome: ", err)
	}
	defer conn.Close()
	defer conn.CloseTarget(ctx)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to open keyboard device: ", err)
	}
	defer kb.Close(ctx)

	defer faillog.DumpUITreeWithScreenshotOnError(
		ctx, s.OutDir(), s.HasError, cr, fmt.Sprintf("%s_dump", s.TestName()))

	const text = "abc"
	if err := ash.SetClipboard(ctx, tconn, text); err != nil {
		s.Fatalf("Failed to add %q to clipboard history: %v", text, err)
	}

	if err := clipboardhistory.PasteAndVerify(tconn, ui, kb, browserui.AddressBarFinder, clipboardhistory.ClipboardHistoryMenuFromShortcut, text, params.pasteType)(ctx); err != nil {
		s.Fatal("Failed to paste from clipboard history: ", err)
	}
}
