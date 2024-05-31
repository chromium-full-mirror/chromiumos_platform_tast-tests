// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package projector is used for writing Projector tests.
package projector

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/familylink"
	"go.chromium.org/tast-tests/cros/local/chrome/projector"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TranscriptTranslation,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests transcript translation for various user types",
		Contacts:     []string{"cros-projector+tast@google.com", "llin@chromium.org"},
		// ChromeOS > Software > Family > Projector
		BugComponent: "b:1080013",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      5 * time.Minute,
		VarDeps: []string{
			"projector.sharedScreencastLink",
		},
		Params: []testing.Param{
			{
				Name:              "regular_consumer",
				Fixture:           "projectorLogin",
				ExtraSoftwareDeps: []string{"gaia"},
			},
			{
				Name:    "supervised_child",
				Fixture: "projectorUnicornLogin",
			},
			{
				Name:              "managed_edu",
				Fixture:           "projectorEduLogin",
				ExtraSoftwareDeps: []string{"gaia"},
			},
		},
	})
}

func TranscriptTranslation(ctx context.Context, s *testing.State) {
	tconn := s.FixtValue().(familylink.HasTestConn).TestConn()
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	sharedScreencast := s.RequiredVar("projector.sharedScreencastLink")

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	// Tast test projector.SharedScreencast already tests opening
	// share links in Lacros, so a Lacros variant is not needed
	// here.
	if err := projector.OpenSharedScreencast(ctx, tconn, cr, browser.TypeAsh, sharedScreencast); err != nil {
		s.Fatal("Failed to open shared screencast: ", err)
	}

	translationDropdown := nodewith.Name("Language").ClassName("translation-lang")
	french := nodewith.Name("français").Role(role.ListBoxOption)
	frenchText := "marks allemands"
	searchToolbar := nodewith.Name("Find in transcript").Role(role.Button)
	searchBox := nodewith.Name("Find in transcript").Role(role.TextField)
	searchResult := nodewith.Name("1/1").Role(role.StaticText).Ancestor(nodewith.ClassName("search-result-label"))
	selectedTranscript := nodewith.Name(frenchText).Role(role.StaticText).Ancestor(nodewith.ClassName("selected"))

	ui := uiauto.New(tconn).WithTimeout(2 * time.Minute)

	if err := uiauto.Combine("translating transcript to French",
		ui.WaitUntilExists(translationDropdown),
		ui.WithInterval(time.Second).LeftClickUntil(translationDropdown, ui.Exists(french)),
		ui.MakeVisible(french),
		ui.FocusAndWait(french),
		ui.WithInterval(time.Second).LeftClickUntil(french, ui.Gone(french)),
		ui.FocusAndWait(searchToolbar),
		ui.WithInterval(time.Second).LeftClickUntil(searchToolbar, ui.Exists(searchBox)),
		ui.LeftClick(searchBox),
		ui.WaitUntilExists(searchBox.Focused()),
	)(ctx); err != nil {
		s.Fatal("Failed to translate transcript to French: ", err)
	}

	// Typing search term into search box.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(ctx)
	if err := kb.Type(ctx, frenchText); err != nil {
		s.Fatal("Failed to type search term: ", err)
	}

	if err := uiauto.Combine("verifying translation",
		// There should only be one search result in the
		// transcript.
		ui.WaitUntilExists(searchResult),
		// We're searching for "German marks" in French so we
		// know translation worked.
		ui.WaitUntilExists(selectedTranscript),
	)(ctx); err != nil {
		s.Fatal("Failed to verify translation: ", err)
	}
}
