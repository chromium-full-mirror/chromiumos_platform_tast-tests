// Copyright 2021 The ChromiumOS Authors
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
		Func:         SelectToSpeak,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "A test that invokes Select-to-Speak and verifies the correct speech is given by the Google TTS engine",
		Contacts: []string{
			"chromeos-a11y-eng@google.com", // Mailing list
			"akihiroota@chromium.org",      // Test author
		},
		BugComponent: "b:1272897",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
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

func SelectToSpeak(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	ed := tts.GoogleTTSEngine()
	text := "This is a select-to-speak test"
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
	expectations := []tts.SpeechExpectation{tts.NewStringExpectation(text)}
	if err := sts.SetSelectionAndActivate(stsData.CTX, cr, textNode, 0, len(text), stsData.SM, expectations); err != nil {
		s.Fatal("Failed to read node: ", err)
	}
}
