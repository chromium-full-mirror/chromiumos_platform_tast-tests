// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"chromiumos/tast/common/chrome/credconfig"
	"chromiumos/tast/common/fixture"
	"chromiumos/tast/common/pci"
	"chromiumos/tast/common/policy"
	"chromiumos/tast/common/policy/fakedms"
	"chromiumos/tast/local/annotations"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/lacros/lacrosfixt"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/policyutil"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PasswordLeakDetectionEnabled,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test PasswordLeakDetectionEnabled policy",
		Contacts: []string{
			"dp-chromeos-eng@google.com",
			"chiav@google.com",
		},
		BugComponent: "b:1129862",
		Attr:         []string{"group:golden_tier"},
		Data:         []string{"password_leak_detection.html"},
		SoftwareDeps: []string{"chrome"},
		VarDeps:      []string{"policy.managedUserAccountPool"},
		Timeout:      3 * time.Minute,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.PasswordLeakDetectionEnabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.SafeBrowsingProtectionLevel{}, pci.VerifiedValue),
		},
		Params: []testing.Param{{
			Fixture: fixture.FakeDMS,
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			Fixture:           fixture.PersistentLacros, // FakeDMS with lacros policy
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               browser.TypeLacros,
		}},
	})
}

// PasswordLeakDetectionEnabled verifies that the PasswordLeakDetectionEnabled policy disables the
// `lookup_single_password_leak` network call. Note that there are a couple potential ways to trigger this:
//  1. Successfully submit a password form
//  2. Call the Credential Management API's `store()` method
//
// We currently use option #2 for this test as it only requires running some JS on a test webpage we control.
func PasswordLeakDetectionEnabled(ctx context.Context, s *testing.State) {
	const (
		annotationID = "16927377" // lookup_single_password_leak

		// Test webpage that stores password credentials using the Credential Management API.
		testFileName = "password_leak_detection.html"
	)

	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	testFilePath := server.URL + "/" + testFileName
	defer server.Close()

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
		chrome.GAIALogin(gaiaCreds), // Real GAIA to enable password leak detection
	}

	// Add lacros chrome opts for lacros runs only.
	if s.Param().(browser.Type) == browser.TypeLacros {
		opts, err = lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(opts...)).Opts()
		if err != nil {
			s.Fatal("Failed to compute lacros chrome options: ", err)
		}
	}

	// Create a new chrome instance.
	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	for _, param := range []struct {
		name                 string
		shouldFindAnnotation bool
		policy               *policy.PasswordLeakDetectionEnabled
	}{
		{
			name:                 "unset",
			shouldFindAnnotation: true,
			policy:               &policy.PasswordLeakDetectionEnabled{Stat: policy.StatusUnset},
		},
		{
			name:                 "enabled",
			shouldFindAnnotation: true,
			policy:               &policy.PasswordLeakDetectionEnabled{Val: true},
		},
		{
			name:                 "disabled",
			shouldFindAnnotation: false,
			policy:               &policy.PasswordLeakDetectionEnabled{Val: false},
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			policies := []policy.Policy{
				param.policy,
				// Enable Safe Browsing, which is required for password leak detection.
				&policy.SafeBrowsingProtectionLevel{Val: 1},
			}
			policyBlob := policy.NewBlob()
			policyBlob.PolicyUser = gaiaCreds.User
			policyBlob.AddPolicies(policies)
			if err := policyutil.ServeBlobAndRefresh(ctx, fdms, cr, policyBlob); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}
			if err := policyutil.Verify(ctx, tconn, policies); err != nil {
				s.Fatal("Failed to verify updated policies: ", err)
			}

			// Setup a browser.
			bt := s.Param().(browser.Type)
			br, closeBrowser, err := browserfixt.SetUp(ctx, cr, bt)
			if err != nil {
				s.Fatal("Failed to open the browser: ", err)
			}
			defer closeBrowser(cleanupCtx)

			// Open the net-export page and start logging.
			if err := annotations.StartLogging(ctx, cr, br); err != nil {
				s.Fatal("Failed to start logging: ", err)
			}

			// Open a webpage that will trigger password leak detection, when enabled.
			conn, err := br.NewConn(ctx, testFilePath)
			if err != nil {
				s.Fatal("Failed to create new Chrome connection: ", err)
			}
			defer conn.Close()
			defer conn.CloseTarget(cleanupCtx)

			// Check the logs for given annotation. Use polling since the password check happens in the background, and
			// there may not always be a UI popup to notify that it is done.
			foundAnnotation := false
			err = testing.Poll(ctx, func(ctx context.Context) error {
				foundAnnotation, err = annotations.CheckLogs(ctx, cr, annotationID)
				if err != nil {
					s.Fatal("Failed to check network logs: ", err)
				}
				if foundAnnotation {
					return nil
				}
				return errors.New("Annotation ID not found yet")
			}, &testing.PollOptions{Timeout: 5 * time.Second})

			// Stop logging.
			if err := annotations.StopLogging(ctx, cr, br); err != nil {
				s.Fatal("Failed to stop logging: ", err)
			}

			// Verify network annotation is present when expected.
			if param.shouldFindAnnotation != foundAnnotation {
				s.Fatalf("Annotation mismatch. Expected: %t. Actual: %t", param.shouldFindAnnotation, foundAnnotation)
			}
		})
	}
}
