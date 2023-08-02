// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package a11y provides functions to assist with interacting with accessibility
// features and settings.
package a11y

import (
	"context"
	"fmt"

	"go.chromium.org/tast-tests/cros/local/a11y/sts"
	"go.chromium.org/tast-tests/cros/local/a11y/tts"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SelectToSpeakMouseSelection,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "A test that invokes Select-to-Speak by holding search and clicking and dragging",
		Contacts: []string{
			"chromeos-a11y-eng@google.com", // Mailing list
			"katie@chromium.org",           // Test author
		},
		BugComponent: "b:1272897", // ChromeOS Public Tracker > Experiences > Accessibility > Features > Select To Speak
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Fixture: "chromeLoggedIn",
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           "lacros",
			Val:               browser.TypeLacros,
		}},
	})
}

func SelectToSpeakMouseSelection(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	ed := tts.GoogleTTSEngine()
	text := "One does not simply walk into Mordor"
	html := fmt.Sprintf("<p>%s</p>", text)
	bt := s.Param().(browser.Type)
	stsData, err := sts.SetUp(ctx, cr, ed, bt, html)
	if err != nil {
		s.Fatal("Failed to set up Select to Speak: ", err)
	}
	defer func() {
		if err := stsData.TDown.TearDown(); err != nil {
			s.Fatal("Failed to tear down Select to Speak test: ", err)
		}
	}()

	rootWebArea := nodewith.Role(role.RootWebArea).First()
	textNode := nodewith.Name(text).Role(role.InlineTextBox).Ancestor(rootWebArea)
	expectations := []tts.SpeechExpectation{tts.NewRegexExpectation(text + "*")}
	if err := sts.ClickAndDragToActivate(stsData.CTX, cr, textNode, stsData.SM, expectations); err != nil {
		s.Fatal("Failed to read node: ", err)
	}
}
