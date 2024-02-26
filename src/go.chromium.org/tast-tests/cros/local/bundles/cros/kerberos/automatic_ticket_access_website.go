// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package kerberos

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/kerberos"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AutomaticTicketAccessWebsite,
		LacrosStatus: testing.LacrosVariantNeeded,
		Desc:         "Checks the behavior of accessing a website secured with Kerberos using the KerberosAccount policy",
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
		},
		VarDeps: []string{"kerberos.username", "kerberos.password", "kerberos.domain"},
		Fixture: fixture.FakeDMS,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.AuthServerAllowlist{}, pci.VerifiedFunctionalityJS),
			pci.SearchFlag(&policy.KerberosAccounts{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.KerberosEnabled{}, pci.VerifiedFunctionalityUI),
		},
	})
}

func AutomaticTicketAccessWebsite(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(*fakedms.FakeDMS)
	username := s.RequiredVar("kerberos.username")
	password := s.RequiredVar("kerberos.password")
	domain := s.RequiredVar("kerberos.domain")
	config := kerberos.ConstructConfig(domain, username)

	kerberosAcc := policy.KerberosAccountsValue{
		Principal: "${LOGIN_ID}" + "@" + domain,
		Password:  "${PASSWORD}",
		Krb5conf:  []string{config.RealmsConfig},
	}

	pb := policy.NewBlob()
	pb.PolicyUser = username + "@managedchrome.com"
	pb.AddPolicies([]policy.Policy{
		&policy.KerberosEnabled{Val: true},
		&policy.AuthServerAllowlist{Val: config.ServerAllowlist},
		&policy.KerberosAccounts{
			Val: []policy.KerberosAccountsValueIf{
				&kerberosAcc,
			},
		},
	})

	if err := fdms.WritePolicyBlob(pb); err != nil {
		s.Fatal("Failed to write policies to FakeDMS: ", err)
	}

	// Start a Chrome instance that will fetch policies from the FakeDMS.
	cr, err := chrome.New(ctx,
		chrome.FakeLogin(chrome.Creds{User: username + "@managedchrome.com", Pass: password}),
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

	defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_automatic_ticket")

	_, err = apps.LaunchOSSettings(ctx, cr, "chrome://os-settings/kerberos")
	if err != nil {
		s.Fatal("Could not open Kerberos section in OS settings: ", err)
	}

	ui := uiauto.New(tconn)

	// Open Kerberos tickets section.
	if err := ui.LeftClick(nodewith.Name("Kerberos tickets").Role(role.Link))(ctx); err != nil {
		s.Fatal("Failed to open Kerberos tickets section: ", err)
	}

	// Trying to find an active ticket.
	if err := kerberos.CheckForTicket(ctx, ui, config); err != nil {
		s.Fatal("Failed to find active ticket: ", err)
	}

	// Close the OS Settings app so the UI locators won't ghost one another.
	apps.Close(ctx, tconn, apps.Settings.ID)

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

	s.Log("Getting the website's title")
	var websiteTitle string
	if err := conn.Eval(ctx, "document.title", &websiteTitle); err != nil {
		s.Fatal("Failed to get the website title: ", err)
	}
	if websiteTitle == "" {
		s.Fatal("Website title is empty")
	}
	if strings.Contains(websiteTitle, "401") {
		s.Error("Website title contains 401")
	}
	if !strings.Contains(websiteTitle, "KerberosTest") {
		s.Fatal("Website title was not KerberosTest but ", websiteTitle)
	}
}
