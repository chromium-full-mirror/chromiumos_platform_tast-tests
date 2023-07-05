// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package kerberos

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/kerberos"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ManualTicketAccessFileSystem,
		// This tast doesn't interact with the browser.
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks the behavior of accessing a file system secured with Kerberos after manually adding a Kerberos ticket",
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
			pci.SearchFlag(&policy.KerberosEnabled{}, pci.VerifiedFunctionalityUI),
		},
	})
}

func ManualTicketAccessFileSystem(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(*fakedms.FakeDMS)
	username := s.RequiredVar("kerberos.username")
	password := s.RequiredVar("kerberos.password")
	domain := s.RequiredVar("kerberos.domain")
	config := kerberos.ConstructConfig(domain, username)

	pb := policy.NewBlob()
	pb.AddPolicies([]policy.Policy{
		&policy.KerberosEnabled{Val: true},
	})

	if err := fdms.WritePolicyBlob(pb); err != nil {
		s.Fatal("Failed to write policies to FakeDMS: ", err)
	}

	cr, err := chrome.New(ctx,
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fdms.URL),
		chrome.KeepEnrollment(),
	)
	if err != nil {
		s.Fatal("Creating Chrome with deferred login failed: ", err)
	}
	defer cr.Close(ctx)

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_manual_ticket")

	// Get a handle to the input keyboard.
	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard handle: ", err)
	}
	defer keyboard.Close(ctx)

	// Change the keyboard layout to English(US). See crbug.com/1351417.
	// If layout is already English(US), which is true for most of the cases,
	// nothing happens.
	ime.EnglishUS.InstallAndActivate(tconn)(ctx)

	ui := uiauto.New(tconn)

	// Add a Kerberos ticket.
	if err := kerberos.AddTicket(ctx, cr, tconn, ui, keyboard, config, password); err != nil {
		s.Fatal("Failed to add Kerberos ticket: ", err)
	}

	// Open the Files App.
	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Launching the Files App failed: ", err)
	}

	s.Log("Mounting SMB share")
	fileShareURLTextBox := nodewith.Name("File share URL").Role(role.TextField)
	if err := uiauto.Combine("add SMB file share",
		files.ClickMoreMenuItem("Services", "SMB file share"),
		ui.WaitForLocation(fileShareURLTextBox),
		keyboard.TypeAction(config.RemoteFileSystemURI),
		ui.LeftClick(nodewith.Name("Add").HasClass("action-button")),
		ui.WaitUntilGone(fileShareURLTextBox),
	)(ctx); err != nil {
		s.Fatal("Failed to add SMB share: ", err)
	}

	if err := uiauto.Combine("wait for SMB to mount and open file",
		files.OpenPath("Files - "+config.Folder, config.Folder),
		files.WaitForFile(config.File),
		files.SelectFile(config.File),
	)(ctx); err != nil {
		s.Fatal("Failed to interact with SMB mount: ", err)
	}
}
