// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package a11y provides functions to assist with interacting with accessibility
// features and settings.
package a11y

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

const (
	testPDFName       = "inaccessible-text.pdf"
	pdfOCRStatusReady = "Image converted to text"
	textInPDFImage    = "Hello, world!"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PDFOCR,
		LacrosStatus: testing.LacrosVariantNeeded,
		Desc:         "Check the PDF OCR feature converts PDF image to text using the OCR (Optical Character Recognition) model available in the screen-ai dlc",
		Contacts: []string{
			"chromeos-a11y-eng@google.com", // Mailing list
			"kyungjunlee@chromium.org",     // Test author
		},
		BugComponent: "b:1272894", // ChromeOS Public Tracker > Experiences > Accessibility > Machine Intelligence
		Attr:         []string{"group:mainline", "informational"},
		Data:         []string{testPDFName}, // Testing PDF containing inaccessible text
		SoftwareDeps: []string{"chrome"},
		Timeout:      5 * time.Minute,
		Params: []testing.Param{{
			Name: "ash",
			Val:  browser.TypeAsh,
		},
		// TODO(b:289009784): Enable this lacros test once b:289080314 is fixed.
		// {
		// 	Name:              "lacros",
		// 	ExtraAttr:         []string{"informational"},
		// 	ExtraSoftwareDeps: []string{"lacros"},
		// 	Val:               browser.TypeLacros,
		// }
		},
	})
}

func PDFOCR(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Setup test HTTP server.
	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	// Launch browser with the feature flag for PDF OCR.
	bt := s.Param().(browser.Type)
	cr, err := browserfixt.NewChrome(ctx, bt, lacrosfixt.NewConfig(),
		chrome.EnableFeatures("PdfOcr"),
	)
	if err != nil {
		s.Fatal("Failed to start chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	if err := a11y.SetFeatureEnabled(ctx, tconn, a11y.SpokenFeedback, true); err != nil {
		s.Fatal("Failed to enable Chromevox: ", err)
	}
	defer a11y.ClearFeature(cleanupCtx, tconn, a11y.SpokenFeedback)

	if err := ossettings.TogglePDFOCR(cr, tconn, true)(ctx); err != nil {
		s.Fatal("Failed to toggle on PDF OCR: ", err)
	}

	// Wait until screen-ai dlc is installed.
	if err := testing.Poll(ctx, a11y.VerifyScreenAIInstalled, &testing.PollOptions{Timeout: 2 * time.Minute, Interval: 10 * time.Second}); err != nil {
		s.Fatal("Failed to wait for screen-ai dlc to be installed: ", err)
	}

	// Open the test PDF.
	conn, _, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, bt, server.URL+"/"+testPDFName)
	if err != nil {
		s.Fatal("Failed to open test PDF: ", err)
	}
	defer closeBrowser(cleanupCtx)
	defer conn.Close()

	ui := uiauto.New(tconn)
	pdfRoot := nodewith.Role(role.PdfRoot)
	status := nodewith.Name(pdfOCRStatusReady).Role(role.Status)
	ocredText := nodewith.Name(textInPDFImage).Role(role.StaticText)

	// Check if PDF OCR successfully extracts text from the inaccessible PDF.
	if err := uiauto.Combine("Check OCR result",
		ui.WithTimeout(30*time.Second).WaitUntilExists(pdfRoot),
		ui.WithTimeout(60*time.Second).WaitUntilExists(status),
		ui.WithTimeout(60*time.Second).WaitUntilExists(ocredText),
	)(ctx); err != nil {
		s.Fatal("Failed to verify text extracted by PDF OCR")
	}
}
