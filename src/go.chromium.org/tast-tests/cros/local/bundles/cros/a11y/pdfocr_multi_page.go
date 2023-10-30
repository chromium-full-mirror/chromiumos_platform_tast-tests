// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package a11y provides functions to assist with interacting with accessibility
// features and settings.
package a11y

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"time"

	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/a11y/chromevox"
	"go.chromium.org/tast-tests/cros/local/a11y/pdfocr"
	"go.chromium.org/tast-tests/cros/local/a11y/tts"
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
		Func:         PDFOCRMultiPage,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Check PDF OCR with a eight-page PDF example",
		Contacts: []string{
			"chromeos-a11y-eng@google.com", // Mailing list
			"kyungjunlee@google.com",       // Test author
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

	if err := ossettings.TogglePDFOCR(cr, tconn, true)(ctx); err != nil {
		s.Fatal("Failed to turn on PDF OCR: ", err)
	}
	// Wait until screen-ai dlc is installed.
	if err := testing.Poll(ctx, a11y.VerifyScreenAIInstalled, &testing.PollOptions{Timeout: 2 * time.Minute, Interval: 10 * time.Second}); err != nil {
		s.Fatal("Failed to wait for screen-ai dlc to be installed: ", err)
	}

	// Open the test PDF.
	conn, _, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, bt, server.URL+"/"+pdfocr.MultiPagePDFName)
	if err != nil {
		s.Fatal("Failed to open test PDF: ", err)
	}
	defer closeBrowser(cleanupCtx)
	defer conn.Close()

	// Get a speech monitor for the Google TTS engine.
	ed := tts.GoogleTTSEngine()
	sm, err := tts.RelevantSpeechMonitor(ctx, cr, tconn, ed)
	if err != nil {
		s.Fatal("Failed to connect to the TTS background page: ", err)
	}
	defer sm.Close()

	ui := uiauto.New(tconn)
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
	jsonData, _ := ioutil.ReadFile(s.DataPath(pdfocr.MultiPagePDFExpectedTextJSONName))
	if err != nil {
		s.Fatal("Failed to open a json file containing expected texts")
	}

	var pages pdfocr.Pages
	if err := json.Unmarshal(jsonData, &pages); err != nil {
		s.Fatal("Failed to unmarshal the json file")
	}

	// Note that ChromeVox was enabled in `pdfocr.SetUp()` above.
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
		if err := tts.PressKeysAndConsumeExpectations(ctx, sm, each.KeyCommands, each.Expectations); err != nil {
			s.Error("Error when pressing keys and expecting speech: ", err)
		}
	}
}
