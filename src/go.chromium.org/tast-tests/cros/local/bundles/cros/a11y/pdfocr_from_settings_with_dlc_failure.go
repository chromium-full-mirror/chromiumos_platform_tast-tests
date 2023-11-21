// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package a11y provides functions to assist with interacting with accessibility
// features and settings.
package a11y

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/a11y/chromevox"
	"go.chromium.org/tast-tests/cros/local/a11y/pdfocr"
	"go.chromium.org/tast-tests/cros/local/a11y/tts"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PDFOCRFromSettingsWithDlcFailure,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Check the PDF OCR feature, being turned on from the Settings, with the screen-ai dlc install failure",
		Contacts: []string{
			"chromeos-a11y-eng@google.com", // Mailing list
			"kyungjunlee@google.com",       // Test author
		},
		BugComponent: "b:1272894", // ChromeOS Public Tracker > Experiences > Accessibility > Machine Intelligence
		Attr:         []string{"group:mainline", "informational"},
		Data:         []string{pdfocr.TestPDFName}, // Testing PDF containing inaccessible text
		SoftwareDeps: []string{"chrome"},
		Timeout:      5 * time.Minute,
		Params: []testing.Param{{
			Name: "ash",
			Val:  browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               browser.TypeLacros,
		}},
	})
}

// PDFOCRFromSettingsWithDlcFailure tests PDF OCR as follows:
// 1. Simulate the screen-ai dlc download failure.
// 2. Turn on PDF OCR from the Settings.
// 3. Check if ChromeVox successfully notifies of this download failure.
// 4.1. 	In the "secondDownloadFailure" scenario:
// 4.1.1. Turn on PDF OCR from the Settings.
// 4.1.2. Check if ChromeVox successfully notifies of this download failure.
// 4.2. 	In the "secondDownloadSuccess" scenario:
// 4.2.1. Refresh DLC by moving the screen-ai dlc to the correct location.
// 4.2.2. Turn on PDF OCR from the Settings.
// 4.2.3. Check if the screen-ai dlc is correctly installed.
// 4.2.4. Check if PDF OCR successfully extracts text from the inaccessible PDF.
func PDFOCRFromSettingsWithDlcFailure(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Setup the dlc failure testing environment for PDF OCR.
	data, err := pdfocr.SetUpDlcFailure(ctx)
	if err != nil {
		// Revert changes made in `pdfocr.SetUpDlcFailure()`.
		if err := data.TDown.TearDown(); err != nil {
			s.Fatal("Failed to tear down PDF OCR dlc failure test: ", err)
		}
		s.Fatal("Failed to set up dlc failure environment for PDF OCR: ", err)
	}
	var skipDeferForDlcFailure bool
	defer func() {
		if skipDeferForDlcFailure {
			return
		}
		if err := data.TDown.TearDown(); err != nil {
			s.Fatal("Failed to tear down PDF OCR dlc failure test: ", err)
		}
	}()

	bt := s.Param().(browser.Type)
	poData, err := pdfocr.SetUpHTTPServer(ctx, cleanupCtx, s.DataFileSystem(), bt)
	if err != nil {
		s.Fatal("Failed to setup PDF OCR test: ", err)
	}
	defer func() {
		if err := poData.TDown.TearDown(); err != nil {
			s.Fatal("Failed to tear down PDF OCR test: ", err)
		}
	}()

	cr := poData.CR
	server := poData.Server

	// Enable ChromeVox and open the test PDF.
	cvData, err := chromevox.SetUpWithURLWithoutFocusWaiter(ctx, cr, tts.GoogleTTSEnUsVoice(), tts.GoogleTTSEngine(), bt, server.URL+"/"+pdfocr.TestPDFName)
	if err != nil {
		s.Fatal("Failed to set up ChromeVox: ", err)
	}
	defer func() {
		if err := cvData.TearDown(); err != nil {
			s.Fatal("Failed to tear down ChromeVox setup: ", err)
		}
	}()

	tconn := cvData.TTSData.TConn
	ui := uiauto.New(tconn)
	ttsHeading := nodewith.NameStartingWith("Text-to-Speech").Role(role.Heading)

	pdfOCRToggle := nodewith.Name(pdfocr.SettingsToggleName).Role(role.ToggleButton)
	for _, subtest := range []struct {
		scenario              string
		secondDownloadSuccess bool
	}{{
		scenario:              "secondDownloadFailure",
		secondDownloadSuccess: false,
	}, {
		scenario:              "secondDownloadSuccess",
		secondDownloadSuccess: true,
	}} {
		s.Run(ctx, subtest.scenario, func(ctx context.Context, s *testing.State) {
			settings, err := ossettings.LaunchAtPageURL(ctx, tconn, cr, "textToSpeech", ui.Exists(ttsHeading))
			if err != nil {
				s.Fatal("Failed to open Text-to-Speech setting page: ", err)
			}
			defer settings.Close(ctx)

			if subtest.secondDownloadSuccess {
				// Restore the screen-ai dlc failure.
				if err := pdfocr.RefreshDlc(ctx, data.BackupDir); err != nil {
					s.Fatal("Failed to restore the screen-ai dlc: ", err)
				}
				skipDeferForDlcFailure = true

				// Turn on PDF OCR always again.
				if err := ossettings.TogglePDFOCR(cr, tconn, true)(ctx); err != nil {
					s.Fatal("Failed to turn on PDF OCR: ", err)
				}
				// Wait until screen-ai dlc is installed.
				if err := testing.Poll(ctx, a11y.VerifyScreenAIInstalled, &testing.PollOptions{Timeout: 2 * time.Minute, Interval: 10 * time.Second}); err != nil {
					s.Fatal("Failed to wait for screen-ai dlc to be installed: ", err)
				}

				// Check if PDF OCR successfully extracts text from the inaccessible PDF.
				pdfRoot := nodewith.Role(role.PdfRoot)
				status := nodewith.Name(pdfocr.StatusReadyMessage).Role(role.Status)
				ocredText := nodewith.Name(pdfocr.TextInPDFImage).Role(role.StaticText)
				if err := uiauto.Combine("Check OCR result",
					ui.WithTimeout(30*time.Second).WaitUntilExists(pdfRoot),
					ui.WithTimeout(30*time.Second).WaitUntilExists(status),
					ui.WithTimeout(30*time.Second).WaitUntilExists(ocredText),
				)(ctx); err != nil {
					s.Fatal("Failed to verify text extracted by PDF OCR")
				}
			} else {
				if err := pdfocr.ExpectDownloadFailureUtterance(cvData.Context(), cvData.SpeechMonitor()); err != nil {
					s.Fatal("Failed to check the ChromeVox announcement for PDF OCR dlc failure: ", err)
				}

				if err := uiauto.Combine("toggle PDF OCR",
					ui.WithTimeout(10*time.Second).WaitUntilExists(pdfOCRToggle),
					// settings.SetToggleOption(cr, pdfocr.SettingsToggleName, true),
					// Failure of screen-ai dlc download makes the PDF OCR toggle button untoggled.
					ui.WithTimeout(60*time.Second).WaitUntilCheckedState(pdfOCRToggle, false),
				)(ctx); err != nil {
					s.Fatal("Failed to wait for the PDF OCR to be reset: ", err)
				}
			}
		})
	}
}
