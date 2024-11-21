// Copyright 2023 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/kerberos"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: InBrowserDialog,
		Desc: "Checks the behavior of Kerberos in-browser dialog",
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
			"group:complementary",
			"group:release-health",
			"release-health_enterprise",
		},
		VarDeps: []string{"kerberos.domain"},
		Fixture: fixture.FakeDMS,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.AuthServerAllowlist{}, pci.VerifiedFunctionalityJS),
			pci.SearchFlag(&policy.KerberosEnabled{}, pci.VerifiedFunctionalityUI),
		},
	})
}

func InBrowserDialog(ctx context.Context, s *testing.State) {
	config := kerberos.ConstructConfig(s.RequiredVar("kerberos.domain") /*username*/, "")
	fdms := s.FixtValue().(*fakedms.FakeDMS)
	pb := policy.NewBlob()
	pb.AddPolicies([]policy.Policy{
		&policy.KerberosEnabled{Val: true},
		&policy.AuthServerAllowlist{Val: config.ServerAllowlist}})

	if err := fdms.WritePolicyBlob(pb); err != nil {
		s.Fatal("Failed to write policies to FakeDMS: ", err)
	}

	// Start a Chrome instance that will fetch policies from the FakeDMS.
	cr, err := chrome.New(ctx,
		chrome.EnableFeatures("KerberosInBrowserRedirect"),
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fdms.URL),
		chrome.KeepEnrollment(),
	)
	if err != nil {
		s.Fatal("Creating Chrome with deferred login failed: ", err)
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
	defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_in_browser_dialog")

	ui := uiauto.New(tconn)
	conn, err := cr.NewConn(ctx, config.WebsiteAddress)
	if err != nil {
		s.Fatal("Failed to connect to chrome: ", err)
	}
	defer conn.Close()

	// The website does not have a valid certificate. We accept the warning and
	// proceed to the content.
	if err := kerberos.ClickAdvancedAndProceed(ctx, conn); err != nil {
		s.Fatal("Could not accept the certificate warning: ", err)
	}

	if err := ui.WaitUntilExists(nodewith.Role(role.Button).HasClass("cancel-button"))(ctx); err != nil {
		s.Fatal("Couldn't find the cancel button: ", err)
	}
	if err := ui.DoDefault(nodewith.Role(role.Button).HasClass("cancel-button"))(ctx); err != nil {
		s.Fatal("Failed to Click the cancel button: ", err)
	}

	// Check that title is 401 - unauthorized.
	if err := kerberos.CheckThatWebsiteTitleIs401(ctx, conn); err != nil {
		s.Fatal("Failed to verify website title: ", err)
	}

	// Refresh the website.
	if err := conn.Navigate(ctx, config.WebsiteAddress); err != nil {
		s.Fatal("Failed to navigate to the server URL")
	}

	if err := ui.WaitUntilExists(nodewith.Role(role.Button).HasClass("action-button"))(ctx); err != nil {
		s.Fatal("Couldn't find the settings button: ", err)
	}
	if err := ui.DoDefault(nodewith.Role(role.Button).HasClass("action-button"))(ctx); err != nil {
		s.Fatal("Failed to click the settings button: ", err)
	}
	if err := ui.WaitUntilExists(nodewith.Role(role.Button).Name("Add a ticket"))(ctx); err != nil {
		s.Fatal("Couldn't find the \"Add a ticket\" button: ", err)
	}
}
