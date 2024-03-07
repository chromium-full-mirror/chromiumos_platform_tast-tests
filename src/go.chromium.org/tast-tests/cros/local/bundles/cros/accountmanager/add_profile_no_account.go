// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package accountmanager provides functions to manage accounts in-session.
package accountmanager

import (
	"context"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/accountmanager"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AddProfileNoAccount,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Addition of a secondary signed-out lacros profile",
		Contacts: []string{
			"team-dent@google.com",
			"emaamari@google.com",
		},
		BugComponent: "b:1279804", // ChromeOS > Software > Commercial (Enterprise) > Identity > Account Manager
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:hw_agnostic",
		},
		SoftwareDeps: []string{"chrome", "lacros"},
		Fixture:      "lacros",
		Timeout:      3 * time.Minute,
	})
}

func AddProfileNoAccount(ctx context.Context, s *testing.State) {
	const newProfileName = "testprofile"

	// Reserve one minute for various cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	// Launch the browser.
	l, err := lacros.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to initialize test: ", err)
	}

	// Set up the keyboard.
	kb, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "add_profile")

	ui := uiauto.New(tconn).WithTimeout(accountmanager.DefaultUITimeout)

	// Browser controls to open a profile.
	profileToolbarButton := nodewith.ClassName("AvatarToolbarButton").Role(role.Button).Focusable()
	profileMenu := nodewith.NameStartingWith("Accounts and sync").Role(role.Menu)
	addProfileButton := nodewith.Name("Add").Role(role.Button).Focusable().Ancestor(profileMenu)

	// Open a new tab.
	conn, err := l.NewConn(ctx, "chrome://version/")
	if err != nil {
		s.Fatal("Failed to open a new tab in Lacros browser: ", err)
	}
	defer conn.Close()

	if err := uiauto.Combine("click a button to add a profile",
		ui.DoDefault(profileToolbarButton),
		ui.DoDefault(addProfileButton),
	)(ctx); err != nil {
		s.Fatal("Failed to click a button to add a profile: ", err)
	}

	// Profile picker screen.
	chooseProfileRoot := nodewith.Name("Choose a profile").Role(role.RootWebArea)
	addButton := nodewith.Name("Add").Role(role.Button).Focusable().Ancestor(chooseProfileRoot)
	if err := ui.Exists(addButton)(ctx); err == nil {
		// If we get profile picker screen - click "Add".
		if err1 := ui.DoDefault(addButton)(ctx); err1 != nil {
			s.Fatal("Failed to click a button to add a profile: ", err1)
		}
	}

	s.Log("Adding a new profile")
	addProfileRoot := nodewith.Name("Set up your new Chrome profile").Role(role.RootWebArea)
	customizeProfileRoot := nodewith.Name("Welcome!").Role(role.Dialog)
	if err := uiauto.Combine("click on nextButton",
		ui.DoDefault(nodewith.Name("Continue without an account").Role(role.Button).Focusable().Ancestor(addProfileRoot)),
		ui.DoDefault(nodewith.NameStartingWith("Rename your profile").Role(role.TextField).Focusable().Required().Ancestor(customizeProfileRoot)),
		kb.TypeAction(newProfileName),
		ui.DoDefault(nodewith.Name("Done").Role(role.Button).Focusable().Ancestor(customizeProfileRoot)),
	)(ctx); err != nil {
		s.Fatal("Failed to click on nextButton: ", err)
	}

	// There are two Chrome windows open. Find the window of the new profile:
	// the name shouldn't contain "About Version" (unlike the first profile).
	newProfileWindow, err := accountmanager.GetChromeProfileWindow(ctx, tconn, func(node uiauto.NodeInfo) bool {
		return !strings.Contains(node.Name, "About Version")
	})
	if err != nil {
		s.Fatal("Failed to find new Chrome window: ", err)
	}

	accountsMenu := nodewith.NameStartingWith("Accounts and sync").Role(role.Menu)
	// Make sure that a new profile was added.
	if err := uiauto.Combine("check that the new profile belongs to the correct account",
		ui.DoDefault(profileToolbarButton.Ancestor(newProfileWindow)),
		ui.WaitUntilExists(nodewith.Name(newProfileName).Role(role.StaticText).Ancestor(accountsMenu)),
	)(ctx); err != nil {
		s.Fatal("Failed to create a new profile for secondary account: ", err)
	}

	// Close all lacros windows and launch lacros again.
	lacros.CloseLacros(ctx, l)
	l, err = lacros.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to initialize lacros: ", err)
	}
	defer lacros.CloseLacros(cleanupCtx, l)

	// Profile picker should be open because there are > 1 profiles.
	if err := ui.WaitUntilExists(chooseProfileRoot)(ctx); err != nil {
		s.Fatal("Failed to open profile picker: ", err)
	}

	// Find all profiles in the profile picker (labels have format "Open X profile").
	profileButton := nodewith.NameRegex(regexp.MustCompile("Open .* profile")).Role(role.Button).Ancestor(chooseProfileRoot)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		profiles, err := ui.NodesInfo(ctx, profileButton)
		if err != nil {
			return errors.Wrap(err, "failed to get profile info")
		}
		if len(profiles) != 2 {
			return errors.Errorf("unexpected number of profiles: got %d, want %d", len(profiles), 2)
		}
		return nil
	}, &testing.PollOptions{Interval: 1 * time.Second, Timeout: 10 * time.Second}); err != nil {
		s.Fatal("Failed to find all profiles: ", err)
	}
}
