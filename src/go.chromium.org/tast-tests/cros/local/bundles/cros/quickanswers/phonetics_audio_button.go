// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package quickanswers

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/quickanswers"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PhoneticsAudioButton,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test Quick Answers phonetics audio button",
		Contacts: []string{
			"assistive-eng@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:905229", // ChromeOS > Software > Assistive
		Attr: []string{
			"group:mainline",
			"informational",
		},
		SoftwareDeps: []string{"chrome", "gaia"},
		Params: []testing.Param{{
			Fixture: quickanswers.Parameterize(
				quickanswers.EnabledWithBrowserFixture,
				quickanswers.VariantSingleWord,
			),
		}, {
			Name: "lacros",
			Fixture: quickanswers.Parameterize(
				quickanswers.EnabledWithBrowserLacrosFixture,
				quickanswers.VariantSingleWord,
			),
			ExtraSoftwareDeps: []string{"lacros"},
		}},
	})
}

// PhoneticsAudioButton tests Quick Answers phonetics audio button.
func PhoneticsAudioButton(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	queryWord := s.FixtValue().(quickanswers.HasQueryWord).QueryWord()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	queryFinder, err := quickanswers.SelectQueryWord(ctx, tconn, queryWord)
	if err != nil {
		s.Fatal("Failed to select a query: ", err)
	}

	// Right click the selected word and ensure the Quick Answers UI shows up with the phonetics audio button.
	// Left click the phonetics audio button to play the audio.
	ui := uiauto.New(tconn)
	quickAnswers := nodewith.ClassName("QuickAnswersView")
	audioButton := nodewith.ClassName("ImageButton").Name("Listen to pronunciation")
	if err := uiauto.Combine("Show context menu",
		ui.RightClick(queryFinder),
		ui.WaitUntilExists(quickAnswers),
		ui.WaitUntilExists(audioButton),
		ui.LeftClick(audioButton))(ctx); err != nil {
		s.Fatal("Quick Answers result not showing up: ", err)
	}

	// Get Current active node.
	cras, err := audio.NewCras(ctx)
	if err != nil {
		s.Fatal("Failed to create Cras object")
	}
	audioDeviceName, _, err := cras.SelectedOutputDevice(ctx)
	if err != nil {
		s.Fatal("Failed to get the selected audio device: ", err)
	}

	// Ensure the phonetics audio is played on the active audio device.
	devName, err := crastestclient.FirstRunningDevice(ctx, audio.OutputStream)
	if err != nil {
		s.Fatal("Failed to detect running output device: ", err)
	}
	if audioDeviceName != devName {
		s.Fatalf("Failed to route the audio through expected audio node: got %q; want %q", devName, audioDeviceName)
	}

	// Dismiss the context menu and ensure the Quick Answers UI also dismiss.
	if err := uiauto.Combine("Dismiss context menu",
		ui.LeftClick(queryFinder),
		ui.WaitUntilGone(quickAnswers))(ctx); err != nil {
		s.Fatal("Quick Answers result not dismissed: ", err)
	}
}
