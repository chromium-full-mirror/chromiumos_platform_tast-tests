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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PDFOCRSinglePage,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test the PDF OCR feature using a single-page PDF",
		Contacts: []string{
			"chrome-screen-ai@google.com", // Mailing list
			"kyungjunlee@google.com",      // Test author
		},
		BugComponent: "b:1272894", // ChromeOS Public Tracker > Experiences > Accessibility > Machine Intelligence
		Attr:         []string{"group:mainline", "informational"},
		Data:         []string{pdfocr.TestPDFName}, // Testing PDF containing inaccessible text
		SoftwareDeps: []string{"chrome"},
		Timeout:      8 * time.Minute,
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

func PDFOCRSinglePage(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	bt := s.Param().(browser.Type)
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

	ui := uiauto.New(cvData.TTSData.TConn)
	pdfRoot := nodewith.Role(role.PdfRoot)
	if err := ui.WaitUntilExists(pdfRoot)(ctx); err != nil {
		s.Fatal("Failed to wait for the PDF ROOT node to be created in the accessibility tree: ", err)
	}

	// PDF OCR is on by default, so wait until screen-ai dlc is installed.
	if err := testing.Poll(ctx, a11y.VerifyScreenAIInstalled, &testing.PollOptions{Timeout: 2 * time.Minute, Interval: 10 * time.Second}); err != nil {
		s.Fatal("Failed to wait for screen-ai dlc to be installed: ", err)
	}

	// PDF OCR is already on, so wait for PDF OCR to finish extracting text from an inaccessible PDF.
	status := nodewith.Name(pdfocr.StatusReadyMessage).Role(role.Status)
	ocredText := nodewith.Name(pdfocr.TextInPDFImage).Role(role.StaticText)
	// Check if PDF OCR successfully extracts text from the inaccessible PDF.
	if err := uiauto.Combine("Check OCR result",
		ui.WithTimeout(30*time.Second).WaitUntilExists(status),
		ui.WithTimeout(30*time.Second).WaitUntilExists(ocredText),
	)(ctx); err != nil {
		s.Fatal("Failed to verify text extracted by PDF OCR")
	}
}
