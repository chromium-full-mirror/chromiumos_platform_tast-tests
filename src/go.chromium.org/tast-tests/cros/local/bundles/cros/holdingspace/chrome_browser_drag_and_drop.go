// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package holdingspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/capturemode"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/holdingspace"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type dragDropParams struct {
	count       int
	browserType browser.Type
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChromeBrowserDragAndDrop,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Tests dragging and dropping files from Holding Space to Chrome Browser",
		BugComponent: "b:1268276", // ChromeOS > Software > System UI Surfaces > HoldingSpace
		Contacts: []string{
			"tote-eng@google.com",
			"cros-system-ui-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"dmblack@google.com",
		},
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-d1cdf1fd-1bf3-42ee-acef-5858f9ceb074",
		}},
		Params: []testing.Param{{
			Name:    "single_file",
			Fixture: "chromeLoggedIn",
			Val: dragDropParams{
				count:       1,
				browserType: browser.TypeAsh,
			},
		}, {
			Name:    "multiple_files",
			Fixture: "chromeLoggedIn",
			Val: dragDropParams{
				count:       2,
				browserType: browser.TypeAsh,
			},
		}, {
			Name:              "single_file_lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           "lacros",
			Val: dragDropParams{
				count:       1,
				browserType: browser.TypeLacros,
			},
		}, {
			Name:              "multiple_files_lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           "lacros",
			Val: dragDropParams{
				count:       2,
				browserType: browser.TypeLacros,
			},
		}},
	})
}

const dragAndDropTestPageURL = `data:text/html, 
<script>
	document.addEventListener('DOMContentLoaded', (event) => {

		function handleDragOver(e) {
			e.preventDefault();
			e.dataTransfer.dropEffect = 'copy';
			return false;
		}

		function handleDrop(e) {
			e.preventDefault();
			([...e.dataTransfer.files]).forEach(displayFileName);
			return false;
		}

		function displayFileName(file) {
			dropArea.append(file.name);
		}

		let dropArea = document.getElementById('drop-area');

		dropArea.addEventListener('dragover', handleDragOver, false);
		dropArea.addEventListener('drop', handleDrop, false);

	});
</script>
<div id="drop-area" style="height: 100%;width: 100%;">
</div>
`

func ChromeBrowserDragAndDrop(ctx context.Context, s *testing.State) {
	params := s.Param().(dragDropParams)
	bt := params.browserType

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr, l, cs, err := lacros.Setup(ctx, s.FixtValue(), bt)
	if err != nil {
		s.Fatal("Failed to initialize test: ", err)
	}
	defer lacros.CloseLacros(cleanupCtx, l)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_dump")

	// Open the html in chrome.
	conn, err := cs.NewConn(ctx, dragAndDropTestPageURL)
	if err != nil {
		s.Fatal("Failed to open new tab: ", err)
	}
	defer conn.Close()

	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user's Download path: ", err)
	}

	// Create number of screenshots needed.
	var screenshotNames []string
	for i := 0; i < params.count; i++ {
		screenshotLocation, err := capturemode.TakeScreenshot(ctx, downloadsPath)
		if err != nil {
			s.Fatal("Failed to capture screenshot: ", err)
		}
		defer os.Remove(screenshotLocation)
		screenshotNames = append(screenshotNames, filepath.Base(screenshotLocation))
	}

	uia := uiauto.New(tconn)

	if err := uia.LeftClick(holdingspace.FindTray())(ctx); err != nil {
		s.Fatal("Failed to click on holding space tray: ", err)
	}
	for _, fileName := range screenshotNames {
		if err := uia.WaitUntilExists(holdingspace.FindScreenCaptureView().Name(fileName))(ctx); err != nil {
			s.Fatal("Failed to verify state after taking screenshot(s): ", err)
		}
	}

	lastFileLocation, err := uia.Location(ctx, holdingspace.FindScreenCaptureView().Name(screenshotNames[len(screenshotNames)-1]))
	if err != nil {
		s.Fatal("Failed to get holding space test file location: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctx)

	leftClickFilesInHoldingSpace := func(ctx context.Context) error {
		for _, fileName := range screenshotNames {
			if err := uia.LeftClick(holdingspace.FindScreenCaptureView().Name(fileName))(ctx); err != nil {
				s.Fatalf("Failed to select %q: %s", fileName, err)
			}
		}
		return nil
	}

	brw, err := ash.FindWindow(ctx, tconn, ash.BrowserTypeMatch(bt))
	if err != nil {
		s.Fatal("Failed to get browser window: ", err)
	}

	if err := uiauto.Combine(fmt.Sprintf("Drag and drop %d screenshots(s) from Holding Space to Chrome Browser", params.count),
		uia.WaitUntilExists(holdingspace.FindRecentFilesBubble()),
		kb.AccelPressAction("Ctrl"),
		leftClickFilesInHoldingSpace,
		kb.AccelReleaseAction("Ctrl"),
		mouse.Drag(tconn, lastFileLocation.CenterPoint(), brw.BoundsInRoot.CenterPoint(), time.Second),
		uia.Gone(holdingspace.FindRecentFilesBubble()),
	)(ctx); err != nil {
		s.Fatalf("Failed to drag and drop %d screenshots(s) from Holding Space to Chrome Browser: %s", params.count, err)
	}

	windowFinder := nodewith.HasClass(brw.Name).Role(role.Window)
	for _, screenshotName := range screenshotNames {
		textFinder := nodewith.Ancestor(windowFinder).Role(role.StaticText).Name(screenshotName)
		if err := uia.WaitUntilExists(textFinder)(ctx); err != nil {
			s.Fatalf("Failed to verify that screenshot %q was dropped into chrome: %s", screenshotName, err)
		}
	}
}
