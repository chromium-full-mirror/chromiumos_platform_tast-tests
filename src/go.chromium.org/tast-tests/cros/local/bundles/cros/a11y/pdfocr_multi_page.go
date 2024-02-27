// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package a11y provides functions to assist with interacting with accessibility
// features and settings.
package a11y

import (
	"context"
	"encoding/json"
	"os"
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
		Func:         PDFOCRMultiPage,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Check PDF OCR with a eight-page PDF example",
		Contacts: []string{
			"chrome-screen-ai@google.com", // Mailing list
			"kyungjunlee@google.com",      // Test author
		},
		BugComponent: "b:1272894", // ChromeOS Public Tracker > Experiences > Accessibility > Machine Intelligence
		Attr:         []string{"group:mainline", "informational"},
		Data: []string{
			pdfocr.MultiPagePDFName,
			pdfocr.MultiPagePDFExpectedTextJSONName,
		},
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

func PDFOCRMultiPage(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	bt := s.Param().(browser.Type)
	// TODO(b/289009784): Create a new helper function that sets up a test environment
	// for PDF OCR using `chromevox.SetUpWithURLWithoutFocusWaiter()`.
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
	cvData, err := chromevox.SetUpWithURLWithoutFocusWaiter(ctx, cr, tts.GoogleTTSEnUsVoice(), tts.GoogleTTSEngine(), bt, server.URL+"/"+pdfocr.MultiPagePDFName)
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

	ui := uiauto.New(cvData.TTSData.TConn)
	pdfRoot := nodewith.Role(role.PdfRoot)
	status := nodewith.Name(pdfocr.StatusReadyMessage).Role(role.Status)
	// Check if PDF OCR successfully extracts text from the inaccessible PDF.
	if err := uiauto.Combine("Check OCR status",
		ui.WithTimeout(30*time.Second).WaitUntilExists(pdfRoot),
		ui.WithTimeout(30*time.Second).WaitUntilExists(status),
	)(ctx); err != nil {
		s.Fatal("Failed to wait for text to be extracted by PDF OCR")
	}

	// Create test steps using expected texts stored in a JSON file.
	jsonData, _ := os.ReadFile(s.DataPath(pdfocr.MultiPagePDFExpectedTextJSONName))
	if err != nil {
		s.Fatal("Failed to open a json file containing expected texts")
	}

	var pages pdfocr.Pages
	if err := json.Unmarshal(jsonData, &pages); err != nil {
		s.Fatal("Failed to unmarshal the json file")
	}

	readingOrder := []pdfocr.TestStep{
		{
			KeyCommands:  []string{chromevox.NextLandmark},
			Expectations: []tts.SpeechExpectation{tts.NewStringExpectation(pdfocr.StatusReadyMessage)},
		},
	}

	for _, page := range pages.Pages {
		// Text extracted from each page is surrouned by a pair of disclaimer nodes.
		readingOrder = append(readingOrder, pdfocr.TestStep{
			KeyCommands:  []string{chromevox.NextLandmark},
			Expectations: []tts.SpeechExpectation{tts.NewStringExpectation(pdfocr.DisclaimerMessageStart)},
		})
		expected := page.Expected
		for _, expectedLine := range expected {
			readingOrder = append(readingOrder, pdfocr.TestStep{
				KeyCommands:  []string{chromevox.NextObject},
				Expectations: []tts.SpeechExpectation{tts.NewRegexExpectation(expectedLine)},
			})
		}
		readingOrder = append(readingOrder, pdfocr.TestStep{
			KeyCommands:  []string{chromevox.NextLandmark},
			Expectations: []tts.SpeechExpectation{tts.NewStringExpectation(pdfocr.DisclaimerMessageEnd)},
		})
	}

	for _, each := range readingOrder {
		if err := tts.PressKeysAndConsumeExpectations(cvData.Context(), cvData.SpeechMonitor(), each.KeyCommands, each.Expectations); err != nil {
			s.Error("Error when pressing keys and expecting speech: ", err)
		}
	}
}
