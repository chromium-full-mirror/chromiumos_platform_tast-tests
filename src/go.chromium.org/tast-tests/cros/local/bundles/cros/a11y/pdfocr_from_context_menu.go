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
	"go.chromium.org/tast-tests/cros/local/a11y/pdfocr"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PDFOCRFromContextMenu,
		LacrosStatus: testing.LacrosVariantNeeded,
		Desc:         "Check the PDF OCR feature by turning it on from the Context Menu",
		Contacts: []string{
			"chromeos-a11y-eng@google.com", // Mailing list
			"kyungjunlee@chromium.org",     // Test author
		},
		BugComponent: "b:1272894", // ChromeOS Public Tracker > Experiences > Accessibility > Machine Intelligence
		Attr:         []string{"group:mainline", "informational"},
		Data:         []string{pdfocr.TestPDFName}, // Testing PDF containing inaccessible text
		SoftwareDeps: []string{"chrome"},
		Timeout:      8 * time.Minute,
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

func PDFOCRFromContextMenu(ctx context.Context, s *testing.State) {
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
		s.Fatal("Failed to start Chrome: ", err)
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

	for _, pdfOCRContextMenu := range []struct {
		name string
	}{{
		name: "Just once",
	}, {
		name: "Always",
	}} {
		s.Run(ctx, pdfOCRContextMenu.name, func(ctx context.Context, s *testing.State) {
			// Open the test PDF.
			conn, _, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, bt, server.URL+"/"+pdfocr.TestPDFName)
			if err != nil {
				s.Fatal("Failed to open test PDF: ", err)
			}
			defer closeBrowser(cleanupCtx)
			defer conn.Close()

			ui := uiauto.New(tconn)
			pdfRoot := nodewith.Role(role.PdfRoot)
			if err := ui.WaitUntilExists(pdfRoot)(ctx); err != nil {
				s.Fatal("Failed to wait for the PDF ROOT node to be created in the accessibility tree: ", err)
			}

			pdfOCRMenuEntry := nodewith.Name("Convert image to text").Role(role.MenuItem)
			pdfOCROption := nodewith.Name(pdfOCRContextMenu.name).Role(role.MenuItem)
			if err := uiauto.Combine("Turn on PDF OCR from the Context Menu",
				ui.WithTimeout(5*time.Second).RightClick(pdfRoot),
				ui.WithTimeout(5*time.Second).LeftClick(pdfOCRMenuEntry),
				ui.WithTimeout(5*time.Second).LeftClick(pdfOCROption),
			)(ctx); err != nil {
				s.Fatal("Failed to turn on PDF OCR from the Context Menu")
			}

			// Wait until screen-ai dlc is installed.
			if err := testing.Poll(ctx, a11y.VerifyScreenAIInstalled, &testing.PollOptions{Timeout: 2 * time.Minute, Interval: 10 * time.Second}); err != nil {
				s.Fatal("Failed to wait for screen-ai dlc to be installed: ", err)
			}

			status := nodewith.Name(pdfocr.StatusReadyMessage).Role(role.Status)
			ocredText := nodewith.Name(pdfocr.TextInPDFImage).Role(role.StaticText)
			// Check if PDF OCR successfully extracts text from the inaccessible PDF.
			if err := uiauto.Combine("Check OCR result",
				ui.WithTimeout(30*time.Second).WaitUntilExists(status),
				ui.WithTimeout(30*time.Second).WaitUntilExists(ocredText),
			)(ctx); err != nil {
				s.Fatal("Failed to verify text extracted by PDF OCR")
			}
		})
	}
}
