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
	"go.chromium.org/tast-tests/cros/local/a11y/pdfocr"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PDFOCRFromSettings,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Check the PDF OCR feature converts PDF image to text using the OCR (Optical Character Recognition) model available in the screen-ai dlc. This test checks the PDF OCR toggle from the Settings",
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

func PDFOCRFromSettings(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	bt := s.Param().(browser.Type)
	data, err := pdfocr.SetUp(ctx, cleanupCtx, s.DataFileSystem(), bt)
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
	tconn := data.TConn

	// Open the test PDF.
	conn, _, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, bt, server.URL+"/"+pdfocr.TestPDFName)
	if err != nil {
		s.Fatal("Failed to open test PDF: ", err)
	}
	defer closeBrowser(cleanupCtx)
	defer conn.Close()

	// PDF OCR is on by default, so the PDF OCR button on the Settings should be already toggled on.
	if err := ossettings.CheckPDFOCRToggle(cr, tconn, true)(ctx); err != nil {
		s.Fatal("Failed to check if PDF OCR is on: ", err)
	}

	if err := testing.Poll(ctx, a11y.VerifyScreenAIInstalled, &testing.PollOptions{Timeout: 2 * time.Minute, Interval: 10 * time.Second}); err != nil {
		s.Fatal("Failed to wait for screen-ai dlc to be installed: ", err)
	}

	ui := uiauto.New(tconn)
	pdfRoot := nodewith.Role(role.PdfRoot)
	status := nodewith.Name(pdfocr.StatusReadyMessage).Role(role.Status)
	ocredText := nodewith.Name(pdfocr.TextInPDFImage).Role(role.StaticText)

	// Check if PDF OCR successfully extracts text from the inaccessible PDF.
	if err := uiauto.Combine("Check OCR result",
		ui.WithTimeout(30*time.Second).WaitUntilExists(pdfRoot),
		ui.WithTimeout(60*time.Second).WaitUntilExists(status),
		ui.WithTimeout(60*time.Second).WaitUntilExists(ocredText),
	)(ctx); err != nil {
		s.Fatal("Failed to verify text extracted by PDF OCR")
	}
}
