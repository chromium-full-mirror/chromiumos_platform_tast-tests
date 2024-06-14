// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package a11y provides functions to assist with interacting with accessibility
// features and settings.
package a11y

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/a11y/chromevox"
	"go.chromium.org/tast-tests/cros/local/a11y/pdfocr"
	"go.chromium.org/tast-tests/cros/local/a11y/tts"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PDFOCRInGalleryApp,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test the PDF OCR feature in the Gallery app",
		Contacts: []string{
			"chrome-screen-ai@google.com", // Mailing list
			"nektar@chromium.org",         // Test author
		},
		BugComponent: "b:1272894", // ChromeOS Public Tracker > Chrome & ChromeOS Accessibility > Accessibility > Machine Intelligence
		Attr:         []string{"group:mainline", "informational"},
		Data:         []string{pdfocr.TestPDFName}, // Testing PDF containing inaccessible text
		SoftwareDeps: []string{"chrome", "chrome_internal"},
		Timeout:      8 * time.Minute,
		Params: []testing.Param{
			{
				Name: "ash",
				Val:  browser.TypeAsh,
			}, {
				Name:              "lacros",
				ExtraSoftwareDeps: []string{"lacros", "lacros_stable"},
				Val:               browser.TypeLacros,
			}},
	})
}

func PDFOCRInGalleryApp(ctx context.Context, s *testing.State) {
	bt := s.Param().(browser.Type)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	data, err := pdfocr.SetUpHTTPServer(ctx, cleanupCtx, s.DataFileSystem(), bt)
	if err != nil {
		s.Fatal("Failed to setup PDF OCR test: ", err)
	}
	defer func() {
		if err := data.TDown.TearDown(); err != nil {
			s.Fatal("Failed to tear down PDF OCR test: ", err)
		}
	}()

	cr := data.CR
	server := data.Server

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	// SWA installation is not guaranteed during startup.
	if err := ash.WaitForChromeAppInstalled(ctx, tconn, apps.Gallery.ID, 2*time.Minute); err != nil {
		s.Fatal("Failed to wait for installed app: ", err)
	}

	// Enable ChromeVox and open the test PDF.
	cvData, err := chromevox.SetUpWithURLWithoutFocusWaiter(ctx, cr, tts.GoogleTTSEnUsVoice(), tts.GoogleTTSEngine(), bt, server.URL)
	if err != nil {
		s.Fatal("Failed to set up ChromeVox: ", err)
	}
	defer func() {
		if err := cvData.TearDown(); err != nil {
			s.Fatal("Failed to tear down ChromeVox setup: ", err)
		}
	}()

	// PDF OCR is on by default, so just wait until screen-ai dlc is installed.
	if err := testing.Poll(ctx, a11y.VerifyScreenAIInstalled, &testing.PollOptions{Timeout: 2 * time.Minute, Interval: 10 * time.Second}); err != nil {
		s.Fatal("Failed to wait for screen-ai dlc to be installed: ", err)
	}

	ui := uiauto.New(cvData.TTSData.TConn).WithInterval(time.Second)

	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user's Download path: ", err)
	}
	// Use the test name to unique name the local test image file.
	// Otherwise the following tests sharing the same Chrome session might have name conflicts.
	// e.g. http://b/198381192.
	localFile := "launch_gallery" + pdfocr.TestPDFName
	localFileLocation := filepath.Join(downloadsPath, localFile)
	if err := ui.Retry(10, func(context.Context) error {
		return fsutil.CopyFile(s.DataPath(pdfocr.TestPDFName), localFileLocation)
	})(ctx); err != nil {
		s.Fatalf("Failed to copy the test image to %s: %s", localFileLocation, err)
	}
	defer os.Remove(localFileLocation)

	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Launching the Files App failed: ", err)
	}

	if err := uiauto.Combine("open Downloads folder and double click file to launch Gallery",
		files.OpenDownloads(),
		files.WithTimeout(30*time.Second).WaitForFile(localFile),
		files.OpenFile(localFile),
	)(ctx); err != nil {
		s.Fatal("Failed to open file in Downloads: ", err)
	}

	if err := ash.WaitForApp(ctx, tconn, apps.Gallery.ID, 1*time.Minute); err != nil {
		s.Fatal("Failed to check Gallery in shelf: ", err)
	}

	// Wait until the Spinner in Gallery App is gone.
	galleryRootFinder := nodewith.NameStartingWith(apps.Gallery.Name).HasClass("BrowserFrame").Role(role.Window).First()
	spinner := nodewith.HasClass("mdc-circular-progress__spinner-layer").Ancestor(galleryRootFinder)
	if err := uiauto.Combine("Wait until the Spinner is gone",
		ui.WithTimeout(3*time.Second).WaitUntilExists(spinner),
		ui.WithTimeout(10*time.Second).WaitUntilGone(spinner),
	)(ctx); err != nil {
		s.Fatal("Failed to wait until the Spinner is gone: ", err)
	}

	// Dismiss the PDF dialog in Gallery App.
	dialog := nodewith.Name("Easily open and edit PDF files").Role(role.Dialog).Ancestor(galleryRootFinder)
	okButton := nodewith.Name("OK").Role(role.Button).Ancestor(dialog)
	if err := uiauto.Retry(3, uiauto.Combine("dismiss PDF dialog",
		ui.WithTimeout(5*time.Second).WaitUntilExists(okButton),
		uiauto.IfSuccessThen(ui.Exists(okButton),
			ui.DoDefaultUntil(okButton,
				ui.WithTimeout(5*time.Second).WaitUntilGone(okButton)),
		)),
	)(ctx); err != nil {
		s.Fatal("Failed to dismiss the PDF dialog: ", err)
	}

	pdfCanvas := nodewith.Role(role.GraphicsDocument).Ancestor(galleryRootFinder)
	pdfRoot := nodewith.Role(role.PdfRoot).Ancestor(pdfCanvas)
	ocredText := nodewith.Name(pdfocr.TextInPDFImage).Role(role.StaticText).Ancestor(pdfRoot)
	// Check if PDF OCR successfully extracts text from the inaccessible PDF.
	if err := uiauto.Combine("Check OCR result",
		ui.WithTimeout(10*time.Second).WaitUntilExists(pdfCanvas),
		ui.WithTimeout(10*time.Second).WaitUntilExists(pdfRoot),
		ui.WithTimeout(10*time.Second).WaitUntilExists(ocredText),
	)(ctx); err != nil {
		s.Fatal("Failed to verify text extracted by PDF OCR: ", err)
	}
}
