// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package kerberos

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/state"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/kerberos"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ManualTicketEditConfiguration,
		// Kerberos integration with Lacros will be covered by the
		// kerberos.ManualTicketAccessWebsite tast.
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that changing Kerberos config works properly",
		Contacts: []string{
			"cros-3pidp@google.com",
			"slutskii@google.com",
			"fsandrade@google.com",
		},
		// ChromeOS > Software > Commercial (Enterprise) > Identity > Active Directory
		BugComponent: "b:1253670",
		SoftwareDeps: []string{"chrome"},
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary"},
		VarDeps: []string{"kerberos.username", "kerberos.password", "kerberos.domain"},
		Fixture: fixture.FakeDMS,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.AuthServerAllowlist{}, pci.VerifiedFunctionalityJS),
			pci.SearchFlag(&policy.KerberosEnabled{}, pci.VerifiedFunctionalityUI),
		},
	})
}

func ManualTicketEditConfiguration(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(*fakedms.FakeDMS)
	username := s.RequiredVar("kerberos.username")
	password := s.RequiredVar("kerberos.password")
	domain := s.RequiredVar("kerberos.domain")
	config := kerberos.ConstructConfig(domain, username)

	// Any value is fine, as long as the number of minutes is not 0
	// and hours number is less than 10, which is the default value.
	var ticketLifetimeConfig = "\nticket_lifetime = 3h37m"
	// Start a Chrome instance that will fetch policies from the FakeDMS.
	cr, err := chrome.New(ctx,
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fdms.URL),
		chrome.KeepEnrollment())
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}

	defer func(ctx context.Context) {
		// Use cr as a reference to close the last started Chrome instance.
		if err := cr.Close(ctx); err != nil {
			s.Error("Failed to close Chrome connection: ", err)
		}
	}(ctx)

	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_manual_ticket")

	// Set Kerberos configuration.
	if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{
		&policy.KerberosEnabled{Val: true},
		&policy.AuthServerAllowlist{Val: config.ServerAllowlist},
	}); err != nil {
		s.Fatal("Failed to update policies: ", err)
	}

	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get a keyboard")
	}
	defer keyboard.Close(ctx)

	// Change the keyboard layout to English(US). See crbug.com/1351417.
	// If layout is already English(US), which is true for most of the cases,
	// nothing happens.
	ime.EnglishUS.InstallAndActivate(tconn)(ctx)

	ui := uiauto.New(tconn)

	if _, err := apps.LaunchOSSettings(ctx, cr, "chrome://os-settings/kerberos"); err != nil {
		s.Fatal("Could not open Kerberos section in OS settings: ", err)
	}

	// Enter Kerberos credentials + open the config.
	if err := uiauto.Combine("open Kerberos configuration",
		ui.LeftClick(nodewith.Name("Kerberos tickets").Role(role.Link)),
		ui.LeftClick(nodewith.Name("Add a ticket").Role(role.Button)),
		ui.LeftClick(nodewith.Name("Kerberos username").Role(role.TextField)),
		keyboard.TypeAction(config.KerberosAccount),
		ui.LeftClick(nodewith.Name("Password").Role(role.TextField)),
		keyboard.TypeAction(password),
		ui.LeftClick(nodewith.Name("Advanced").Role(role.Link)),
	)(ctx); err != nil {
		s.Fatal("Failed to open configuration menu: ", err)
	}

	configFinder := nodewith.Role(role.TextField).State(state.Editable, true).State(state.Multiline, true)

	// Save the default kerberos configuration, even if it's empty.
	node, err := ui.Info(ctx, configFinder)
	if err != nil {
		s.Fatal(err, "Not able to read the default configuration")
	}
	var defaultConfig = node.Value

	if err := uiauto.Combine("type configuration and Cancel",
		ui.LeftClick(configFinder),
		keyboard.TypeAction(ticketLifetimeConfig),
		ui.LeftClick(nodewith.Name("Cancel").Role(role.Button)),
		ui.LeftClick(nodewith.Name("Advanced").Role(role.Link)),
	)(ctx); err != nil {
		s.Fatal("Failed to open configuration menu: ", err)
	}

	node, err = ui.Info(ctx, configFinder)
	if err != nil {
		s.Fatal("Not able to read the configuration: ", err)
	}

	// Check that the configuration has not changed, since Cancel was pressed.
	if node.Value != defaultConfig {
		s.Fatal("The Cancel button didn't delete new configuration")
	}

	// Create a ticket with custom lifetime and check if it was actually added.
	if err := uiauto.Combine("change lifetime of Kerberos ticket",
		ui.LeftClick(configFinder),
		keyboard.TypeAction(ticketLifetimeConfig),
		ui.LeftClick(nodewith.Name("Save").Role(role.Button)),
		ui.LeftClick(nodewith.Name("Add").HasClass("action-button")),
		ui.WaitUntilExists(nodewith.NameContaining("Valid for 3 hours").Role(role.StaticText)),
	)(ctx); err != nil {
		s.Fatal("Failed to create a ticket with custom lifetime: ", err)
	}

	// Check that configuration can not be saved if the syntax is wrong.
	if err := uiauto.Combine("open configuration menu and add invalid configuration",
		ui.LeftClick(nodewith.HasClass("icon-more-vert more-actions").Role(role.Button)),
		ui.LeftClick(nodewith.Name("Refresh now").Role(role.MenuItem)),
		ui.LeftClick(nodewith.Name("Advanced").Role(role.Link)),
		ui.LeftClick(configFinder),
		keyboard.TypeAction("\n[realms"), // syntax error, no closing bracket for "["
		ui.LeftClick(nodewith.Name("Save").Role(role.Button)),
		ui.WaitUntilExists(nodewith.NameContaining("syntax error").Role(role.StaticText)),
		ui.LeftClick(nodewith.Name("Cancel").Role(role.Button)),
	)(ctx); err != nil {
		s.Fatal("Failed to find syntax error: ", err)
	}

	if err := uiauto.Combine("add invalid configuration",
		ui.LeftClick(nodewith.Name("Advanced").Role(role.Link)),
		ui.LeftClick(configFinder),
		keyboard.TypeAction("\nticket_lifetime: 1337h"), // syntax error, should be "=" instead of ":"
		ui.LeftClick(nodewith.Name("Save").Role(role.Button)),
		ui.WaitUntilExists(nodewith.NameContaining("syntax error").Role(role.StaticText)),
		ui.LeftClick(nodewith.Name("Cancel").Role(role.Button)),
	)(ctx); err != nil {
		s.Fatal("Failed to find syntax error: ", err)
	}

	// Check that configuration can not be saved if some options are blocklisted.
	if err := uiauto.Combine("add invalid configuration",
		ui.LeftClick(nodewith.Name("Advanced").Role(role.Link)),
		ui.LeftClick(configFinder),
		keyboard.TypeAction("\nallow_weak_crypto = true"), // "allow_weak_crypto = true" is blocklisted
		ui.LeftClick(nodewith.Name("Save").Role(role.Button)),
		ui.WaitUntilExists(nodewith.NameContaining("option not supported").Role(role.StaticText)),
	)(ctx); err != nil {
		s.Fatal("Failed to find blocklist error: ", err)
	}
}
