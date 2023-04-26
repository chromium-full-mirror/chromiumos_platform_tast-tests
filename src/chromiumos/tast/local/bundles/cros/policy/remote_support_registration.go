// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"

	"chromiumos/tast/common/chrome/credconfig"
	"chromiumos/tast/common/fixture"
	"chromiumos/tast/common/pci"
	"chromiumos/tast/common/policy"
	"chromiumos/tast/common/policy/fakedms"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/lacros/lacrosfixt"
	"chromiumos/tast/local/chrome/uiauto/crd"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/policyutil"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RemoteSupportRegistration,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verifies behavior of RemoteAccessHostAllowRemoteSupportConnections policy",
		Contacts: []string{
			"dp-chromeos-eng@google.com",
			"crmullins@google.com",
		},
		BugComponent: "b:4617222",
		Attr:         []string{"group:golden_tier"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"policy.managedUserAccountPool"},
		Timeout:      3 * time.Minute,
		Params: []testing.Param{{
			Fixture: fixture.FakeDMS,
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           fixture.PersistentLacros, // FakeDMS with lacros policy
			Val:               browser.TypeLacros,
		}},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.RemoteAccessHostAllowRemoteSupportConnections{},
				pci.VerifiedFunctionalityUI),
		},
	})
}

func RemoteSupportRegistration(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	gaiaCreds, err := credconfig.PickRandomCreds(
		s.RequiredVar("policy.managedUserAccountPool"))
	if err != nil {
		s.Fatal("Failed to parse managed user creds: ", err)
	}

	policyBlob := policy.NewBlob()
	policyBlob.PolicyUser = gaiaCreds.User
	if err := fdms.WritePolicyBlob(policyBlob); err != nil {
		s.Fatal("Failed to write policies to FakeDMS: ", err)
	}

	opts := []chrome.Option{
		chrome.DMSPolicy(fdms.URL),  // FakeDMS for setting policies
		chrome.GAIALogin(gaiaCreds), // Real GAIA to enable CRD
	}

	if s.Param().(browser.Type) == browser.TypeLacros {
		opts, err = lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(opts...)).Opts()
		if err != nil {
			s.Fatal("Failed to compute lacros chrome options: ", err)
		}
	}

	// Shorten the context to make room for cleanup jobs.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	for _, param := range []struct {
		name                   string
		shouldCrdLaunchSucceed bool
		policy                 *policy.RemoteAccessHostAllowRemoteSupportConnections
	}{
		{
			name:                   "unset",
			shouldCrdLaunchSucceed: true,
			policy:                 &policy.RemoteAccessHostAllowRemoteSupportConnections{Stat: policy.StatusUnset},
		},
		{
			name:                   "enabled",
			shouldCrdLaunchSucceed: true,
			policy:                 &policy.RemoteAccessHostAllowRemoteSupportConnections{Val: true},
		},
		{
			name:                   "disabled",
			shouldCrdLaunchSucceed: false,
			policy:                 &policy.RemoteAccessHostAllowRemoteSupportConnections{Val: false},
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {

			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			policies := []policy.Policy{param.policy}
			policyBlob := policy.NewBlob()
			policyBlob.PolicyUser = gaiaCreds.User
			policyBlob.AddPolicies(policies)
			if err := policyutil.ServeBlobAndRefresh(ctx, fdms, cr, policyBlob); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}
			if err := policyutil.Verify(ctx, tconn, policies); err != nil {
				s.Fatal("Failed to verify updated policies: ", err)
			}

			// Setup browser.
			br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
			if err != nil {
				s.Fatal("Failed to open the browser: ", err)
			}
			defer closeBrowser(cleanupCtx)
			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			didCrdLaunchSucceed := true
			errContainsRemoteSupportBlockedMessage := false
			if err := crd.Launch(ctx, br, tconn); err != nil {
				didCrdLaunchSucceed = false
				errContainsRemoteSupportBlockedMessage = strings.Contains(err.Error(), "Remote support connections blocked")
			}

			if param.shouldCrdLaunchSucceed && didCrdLaunchSucceed == false {
				s.Fatal("Remote desktop unexpectedly failed")
			}

			if !param.shouldCrdLaunchSucceed && didCrdLaunchSucceed == true {
				s.Fatal("Remote desktop succeeded when expected to fail")
			}

			if !param.shouldCrdLaunchSucceed && errContainsRemoteSupportBlockedMessage == false {
				s.Fatal("Remote desktop failure message did not include connections blocked")
			}
		})
	}

}
