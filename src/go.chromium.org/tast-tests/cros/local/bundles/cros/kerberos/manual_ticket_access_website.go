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
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/kerberos"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ManualTicketAccessWebsite,
		Desc: "Checks the behavior of accessing a website secured by Kerberos after manually adding a Kerberos ticket",
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

func ManualTicketAccessWebsite(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(*fakedms.FakeDMS)
	username := s.RequiredVar("kerberos.username")
	password := s.RequiredVar("kerberos.password")
	domain := s.RequiredVar("kerberos.domain")
	config := kerberos.ConstructConfig(domain, username)

	// Start a Chrome instance that will fetch policies from the FakeDMS.
	cr, err := chrome.New(ctx,
		// TODO(b/260522530): remove this after KerberosInBrowser feature
		// is launched (launch/4210638).
		chrome.DisableFeatures("KerberosInBrowserRedirect"),
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

	// Check that title is 401 - unauthorized.
	if err := kerberos.CheckThatWebsiteTitleIs401(ctx, conn); err != nil {
		s.Fatal("Failed to verify website title: ", err)
	}

	// Access keyboard.
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

	// Add a Kerberos ticket.
	if err := kerberos.AddTicket(ctx, cr, tconn, ui, keyboard, config, password); err != nil {
		s.Fatal("Failed to add Kerberos ticket: ", err)
	}

	// Sometimes the window refreshes too fast and Kerberos settings are not yet applied
	// so this part is done until success/timeout.
	var websiteTitle string
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// Refresh the website.
		if err := conn.Navigate(ctx, config.WebsiteAddress); err != nil {
			return errors.New("failed to navigate to the server URL")
		}
		// Wait for the website to load.
		if err := conn.WaitForExpr(ctx, "document.readyState === 'complete'"); err != nil {
			errors.Wrap(err, "failed waiting for URL to load")
		}
		if err := conn.Eval(ctx, "document.title", &websiteTitle); err != nil {
			return errors.Wrap(err, "failed to get the website title")
		}
		if websiteTitle == "" {
			return errors.New("Website title is empty")
		}
		if strings.Contains(websiteTitle, "401") {
			return errors.New("Website title contains 401")
		}
		if !strings.Contains(websiteTitle, "KerberosTest") {
			return errors.New("Website title was not KerberosTest but ")
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  10 * time.Second,
		Interval: 1 * time.Second,
	}); err != nil {
		s.Fatal("Failed to get access to the website: ", err)
	}
}
