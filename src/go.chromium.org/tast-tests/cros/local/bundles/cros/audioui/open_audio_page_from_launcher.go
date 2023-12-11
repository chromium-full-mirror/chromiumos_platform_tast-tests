// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audioui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OpenAudioPageFromLauncher,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Audio settings page can be found and launched from the launcher",
		// ChromeOS > Software > System Services > Peripherals > Audio (1321112)
		BugComponent: "b:1321112",
		Contacts: []string{
			"cros-peripherals@google.com",
			"ashleydp@google.com",
			"zentaro@google.com",
		},
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		SearchFlags: []*testing.StringPair{
			{
				Key:   "feature_id",
				Value: "screenplay-816eefa8-76ad-43ec-8300-c747f4b59987",
			},
		},
	})
}

// OpenAudioPageFromLauncher uses launcher search bar to open Audio Settings.
func OpenAudioPageFromLauncher(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Open chrome with audio settings enabled.
	cr, err := chrome.New(ctx, chrome.EnableFeatures("AudioSettingsPage"))
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	// Close test instance of Chrome.
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}
	defer launcher.HideLauncher(tconn, false)
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(ctx)

	const query = "audio"
	testing.ContextLogf(ctx, "Searching for query: (%q) using launcher search", query)
	ui := uiauto.New(tconn).WithTimeout(10 * time.Second)
	// Search for audio using launcher and attempt to click result.
	audioSearchItem := launcher.SearchResultListItemFinder.NameContaining("Audio, Device, Settings")
	if err := uiauto.Combine("Search for audio",
		launcher.Open(tconn),
		launcher.Search(tconn, kb, query),
		ui.WaitUntilExists(audioSearchItem),
		ui.DoDefault(audioSearchItem),
	)(ctx); err != nil {
		s.Fatalf("Failed to search query (%q) and click result (%p)", query, audioSearchItem)
	}
	testing.ContextLogf(ctx, "Clicked query result matching (%p)", audioSearchItem)

	// Verify OS Settings opened to expected page.
	osAudioSettingsWindow := nodewith.Role(role.RootWebArea).NameContaining("Settings - Audio").First()
	if err := ui.WithTimeout(20 * time.Second).WaitUntilExists(osAudioSettingsWindow)(ctx); err != nil {
		s.Fatal("Failed find audio settings window")
	}
	testing.ContextLog(ctx, "Verified Audio settings opened")
}
