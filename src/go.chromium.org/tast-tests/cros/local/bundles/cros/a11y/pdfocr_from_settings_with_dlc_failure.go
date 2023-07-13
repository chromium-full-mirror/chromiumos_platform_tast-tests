// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package a11y provides functions to assist with interacting with accessibility
// features and settings.
package a11y

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/a11y/pdfocr"
	"go.chromium.org/tast-tests/cros/local/a11y/tts"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PDFOCRFromSettingsWithDlcFailure,
		LacrosStatus: testing.LacrosVariantNeeded,
		Desc:         "Check the PDF OCR feature, being turned on from the Settings, with the screen-ai dlc install failure",
		Contacts: []string{
			"chromeos-a11y-eng@google.com", // Mailing list
			"kyungjunlee@chromium.org",     // Test author
		},
		BugComponent: "b:1272894", // ChromeOS Public Tracker > Experiences > Accessibility > Machine Intelligence
		Attr:         []string{"group:mainline", "informational"},
		Data:         []string{pdfocr.TestPDFName}, // Testing PDF containing inaccessible text
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

func PDFOCRFromSettingsWithDlcFailure(ctx context.Context, s *testing.State) {
	// Setup the dlc failure testing environment for PDF OCR.
	data, err := pdfocr.SetUpDlcFailure(ctx)
	if err != nil {
		s.Fatal("Failed to set up dlc failure environment for PDF OCR: ", err)
	}
	defer func() {
		if err := data.TDown.TearDown(); err != nil {
			s.Fatal("Failed to tear down PDF OCR dlc failure test: ", err)
		}
	}()

	ctx = data.CTX
	cleanupCtx := data.CleanupCTX
	bt := s.Param().(browser.Type)
	poData, err := pdfocr.SetUp(ctx, cleanupCtx, s.DataFileSystem(), bt)
	if err != nil {
		s.Fatal("Failed to setup PDF OCR test: ", err)
	}
	defer func() {
		if err := poData.TDown.TearDown(); err != nil {
			s.Fatal("Failed to tear down PDF OCR test: ", err)
		}
	}()

	cr := poData.CR
	tconn := poData.TConn

	// Get a speech monitor for the Google TTS engine.
	ed := tts.GoogleTTSEngine()
	sm, err := tts.RelevantSpeechMonitor(ctx, cr, tconn, ed)
	if err != nil {
		s.Fatal("Failed to connect to the TTS background page: ", err)
	}
	defer sm.Close()

	ui := uiauto.New(tconn)

	ttsHeading := nodewith.NameStartingWith("Text-to-Speech").Role(role.Heading)
	settings, err := ossettings.LaunchAtPageURL(ctx, tconn, cr, "textToSpeech", ui.Exists(ttsHeading))
	if err != nil {
		s.Fatal("Failed to open Text-to-Speech setting page: ", err)
	}
	defer settings.Close(ctx)

	pdfOCRToggle := nodewith.Name(pdfocr.SettingsToggleName).Role(role.ToggleButton)
	if err := uiauto.Combine("toggle PDF OCR",
		ui.WithTimeout(10*time.Second).WaitUntilExists(pdfOCRToggle),
		settings.SetToggleOption(cr, "Convert PDF images to text", true),
		// Failure of screen-ai dlc download makes the PDF OCR toggle button untoggled.
		ui.WithTimeout(50*time.Second).WaitUntilCheckedState(pdfOCRToggle, false),
	)(ctx); err != nil {
		s.Fatal("Failed to wait for the PDF OCR to be reset: ", err)
	}

	if err := sm.Consume(ctx, []tts.SpeechExpectation{
		tts.NewStringExpectation("Downloading text recognition files"),
		tts.NewRegexExpectation("Can't download text recognition files*"),
	}); err != nil {
		s.Fatal("Failed to check the ChromeVox announcement for PDF OCR dlc failure: ", err)
	}
}
