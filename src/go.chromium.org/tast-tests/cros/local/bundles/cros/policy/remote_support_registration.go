// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/annotations"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/remotedesktop"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
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
	isLacros := s.Param().(browser.Type) == browser.TypeLacros

	gaiaCreds, err := credconfig.PickRandomCreds(
		s.RequiredVar("policy.managedUserAccountPool"))
	if err != nil {
		s.Fatal("Failed to parse managed user creds: ", err)
	}

	// Set up keyboard.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctx)

	policyBlob := policy.NewBlob()
	policyBlob.PolicyUser = gaiaCreds.User
	if err := fdms.WritePolicyBlob(policyBlob); err != nil {
		s.Fatal("Failed to write policies to FakeDMS: ", err)
	}

	opts := []chrome.Option{
		chrome.DMSPolicy(fdms.URL),  // FakeDMS for setting policies
		chrome.GAIALogin(gaiaCreds), // Real GAIA to enable CRD
		chrome.ExtraArgs("--force-devtools-available"),
	}

	if isLacros {
		opts = append(opts, chrome.LacrosExtraArgs("--force-devtools-available"))
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

	for key, param := range remotedesktop.TestCases() {
		s.Run(ctx, param.Name, func(ctx context.Context, s *testing.State) {

			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			policies := []policy.Policy{param.Policy}
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
			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.Name)

			// These network calls are only made by the host in the lacros environment.
			// In the ash clients these calls are handled by the website instead.
			// So we only perform this check for lacros clients.
			if isLacros {
				if err := annotations.StartOSLogging(ctx, cr, br, kb); err != nil {
					s.Fatal("Failed to start logging: ", err)
				}
			}

			if err := remotedesktop.TriggerRemoteSupportRegistration(ctx, nil, br, nil, tconn, key); err != nil {
				s.Fatal("Failure during CRD launch: ", err)
			}

			hashCodes := []string{
				remotedesktop.FTLMessagingClientReceiveMessagesHashCode,
				remotedesktop.FTLRegistrationManagerHashCode,
				remotedesktop.RemotingRegisterSupportHostRequestHashCode,
			}
			if isLacros {
				if _, err := annotations.StopOSLoggingVerifyAnnotationSet(ctx, cr, br, kb, param.ShouldFindAnnotations, hashCodes); err != nil {
					s.Fatal("Failed to stop OS logging and verify logs: ", err)
				}
			}
		})
	}

}
