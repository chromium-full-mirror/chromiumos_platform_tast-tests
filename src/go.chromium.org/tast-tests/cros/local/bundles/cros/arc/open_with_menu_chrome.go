// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OpenWithMenuChrome,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test ARC's open with menu show up in Chrome's right click menu",
		Contacts:     []string{"cros-arc-te@google.com", "lacros-fundamentals@google.com", "elkurin@chromium.org"},
		// ChromeOS > Software > ARC++ > Core
		BugComponent: "b:488493",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container"},
			Fixture:           "arcBooted",
			Val:               browser.TypeAsh,
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
			Fixture:           "arcBooted",
			Val:               browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"android_p", "lacros"},
			Fixture:           "lacrosWithArcBooted",
			Val:               browser.TypeLacros,
		}, {
			Name:              "lacros_vm",
			ExtraSoftwareDeps: []string{"android_vm", "lacros"},
			Fixture:           "lacrosWithArcBooted",
			Val:               browser.TypeLacros,
		}},
	})
}

func OpenWithMenuChrome(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*arc.PreData).Chrome
	arcDevice := s.FixtValue().(*arc.PreData).ARC

	const (
		appName         = "Intent Picker Test App"
		testPackageName = "org.chromium.arc.testapp.chromeintentpicker"
		intentActionID  = testPackageName + ":id/intent_action"
		expectedAction  = "android.intent.action.VIEW"
	)

	// Give 5 seconds to clean up and dump out UI tree.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)

	// Setup ARC and Installs APK.
	if err := arcDevice.WaitIntentHelper(ctx); err != nil {
		s.Fatal("Failed to wait for ARC Intent Helper: ", err)
	}

	if err := arcDevice.Install(ctx, arc.APKPath("ArcChromeIntentPickerTest.apk")); err != nil {
		s.Fatal("Failed installing the APK: ", err)
	}

	// Setup the app link verification for the test app as verified.
	version, err := arc.SDKVersion()
	if err != nil {
		s.Fatal("Failed to get ARC version: ", err)
	}

	if version >= arc.SDKT {
		if err := arcDevice.Command(ctx, "pm", "set-app-links", "--package", testPackageName, "2", "all").Run(testexec.DumpLogOnError); err != nil {
			s.Fatal("Failed to set Android link capturing setting: ", err)
		}
	} else {
		if err := arcDevice.Command(ctx, "pm", "set-app-link", testPackageName, "always").Run(testexec.DumpLogOnError); err != nil {
			s.Fatal("Failed to set Android link capturing setting: ", err)
		}
	}

	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
	if err != nil {
		s.Fatal("Failed to open the browser: ", err)
	}
	defer closeBrowser(cleanupCtx)

	// Open search result page
	conn, err := br.NewConn(ctx, "https://google.com/search?q=Google")
	if err != nil {
		s.Fatal("Failed to create new Chrome connection: ", err)
	}
	defer conn.Close()

	// Wait for Google Logo to appear
	googleLogo := nodewith.Name("Google").Role(role.Link).First()
	if err := ui.WaitUntilExists(googleLogo)(ctx); err != nil {
		s.Fatal("Failed to wait for Google Logo to load: ", err)
	}

	// Get the Google Logo next to search bar.
	googleLogoOption := nodewith.Name("Open with Intent Picker Test App").Role(role.MenuItem)
	if err := uiauto.Combine("Show context menu",
		ui.RightClick(googleLogo),
		ui.WaitUntilExists(googleLogoOption))(ctx); err != nil {
		s.Log("Failed to show context menu of Google Logo: ", err)
		// After timeout, dump all the menuItems if possible, this should provide a clear
		// idea whether items are missing in the menu or the menu not being there at all.
		menu := nodewith.ClassName("MenuItemView")
		menuItems, err := ui.NodesInfo(ctx, menu)
		if err != nil {
			s.Fatal("Could not find context menu items: ", err)
		}
		var items []string
		for _, item := range menuItems {
			items = append(items, item.Name)
		}
		s.Fatalf("Found %d menu items, including: %s", len(items), strings.Join(items, " / "))
	}
}
