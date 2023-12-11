// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package holdingspace

import (
	"context"
	"os"
	"path/filepath"
	"regexp"

	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/holdingspace"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/printpreview"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type printToPdfParams struct {
	browserType browser.Type
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         PrintToPDF,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verifies print to pdf file appears in holding space",
		BugComponent: "b:1268276", // ChromeOS > Software > System UI Surfaces > HoldingSpace
		Contacts: []string{
			"tote-eng@google.com",
			"cros-system-ui-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"dmblack@google.com",
		},
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-cbd2ebb4-8f09-4902-a7ae-9eb7619f7409",
		}},
		Params: []testing.Param{{
			Name: "ash",
			Val: printToPdfParams{
				browserType: browser.TypeAsh,
			},
		}, {
			Name: "lacros",
			Val: printToPdfParams{
				browserType: browser.TypeLacros,
			},
			ExtraSoftwareDeps: []string{"lacros"},
		}},
	})
}

// PrintToPDF verifies that after printing to pdf, the file is displayed in the Downloads
// section of Holding Space.
func PrintToPDF(ctx context.Context, s *testing.State) {
	params := s.Param().(printToPdfParams)
	bt := params.browserType

	cr, br, closeBrowser, err := browserfixt.SetUpWithNewChrome(ctx, bt, lacrosfixt.NewConfig())
	if err != nil {
		s.Fatal("Failed to connect to Chrome: ", err)
	}
	defer cr.Close(ctx)
	defer closeBrowser(ctx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "print_to_pdf")

	// Open a new Chrome window with an empty browser tab for us to to test printing to
	// pdf with.
	conn, err := br.NewConn(ctx, "")
	if err != nil {
		s.Fatal("Failed to create a new Chrome window: ", err)
	}
	defer conn.Close()

	// Wait for the tab to load.
	const expectedTabTitle = "about:blank"
	_, err = ash.FindOnlyWindow(ctx, tconn, func(w *ash.Window) bool {
		return (bt == browser.TypeAsh && w.WindowType == ash.WindowTypeBrowser) ||
			(bt == browser.TypeLacros && w.WindowType == ash.WindowTypeLacros) &&
				w.IsActive &&
				regexp.MustCompile(expectedTabTitle).MatchString(w.Title)
	})
	if err != nil {
		s.Fatalf("Failed to find active window with title having %q as a substring: %v",
			expectedTabTitle, err)
	}

	// Defer cleanup of the downloaded PDF file.
	var fileName string
	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user's Download path: ", err)
	}
	defer func() {
		if fileName != "" {
			downloadLocation := filepath.Join(downloadsPath, fileName)
			os.Remove(downloadLocation)
		}
	}()

	ui := uiauto.New(tconn)
	kb, err := input.Keyboard(ctx)
	if err := uiauto.Combine("Save file as PDF and verify presence in holding space",
		// Open print preview using the Ctrl+P shortcut.
		kb.AccelAction("Ctrl+P"),
		printpreview.WaitForPrintPreview(tconn),

		// Select "Save as PDF" as the printer option.
		func(ctx context.Context) error {
			return printpreview.SelectPrinter(ctx, tconn, "Save as PDF")
		},
		printpreview.WaitForPrintPreview(tconn),

		// Click the "Save" button.
		ui.LeftClick(nodewith.Name("Save").Role(role.Button)),

		// Download file window will popup.
		// Wait for the input field to load with a default file name.
		ui.RetrySilently(8, func(ctx context.Context) error {
			nodeInfo, err := ui.Info(ctx, nodewith.Name("File name").Role(role.TextField))
			if err != nil {
				return err
			}

			if !(filepath.Ext(nodeInfo.Value) == ".pdf") {
				return errors.Errorf("file name %q should contain .pdf", nodeInfo.Value)
			}

			// Use the default file name of the PDF file we're about to save.
			// We do this rather than typing one as we were seeing different file names on different devices in the Files app.
			// e.g. some devices had an additional ".pdf" appended on to whatever was typed, even if we cleared the field first.
			fileName = nodeInfo.Value
			return nil
		}),

		ui.LeftClick(nodewith.Name("Save").
			Role(role.Button).
			Ancestor(nodewith.Name("Save file as").Role(role.Window))),

		// Left click the tray to open the holding space bubble.
		ui.LeftClick(holdingspace.FindTray()),

		// Check the downloaded PDF is there.
		// Wrapping the action `ui.WaitUntilExists` in another action so that `fileName` isn't evaluated until runtime.
		func(ctx context.Context) error {
			return ui.WaitUntilExists(holdingspace.FindChip().Name(fileName))(ctx)
		},
	)(ctx); err != nil {
		s.Fatal("Failed to save as PDF and verify presence in holding space: ", err)
	}
}
