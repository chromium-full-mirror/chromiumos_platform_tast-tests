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
	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/networkrequestmonitor"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/wallpapergooglephotos"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/netexport"
	"go.chromium.org/tast-tests/cros/local/policyutil"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         WallpaperGooglePhotosIntegrationEnabled,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies the behavior of the WallpaperGooglePhotosIntegrationEnabled policy",
		Contacts: []string{
			"dp-chromeos-eng@google.com",
			"crmullins@google.com",
		},
		BugComponent: "b:1129862",
		Attr: []string{
			"group:golden_tier",
			"group:hardware",
			"group:hw_agnostic",
		},
		SoftwareDeps: []string{"chrome", "gaia"},
		VarDeps:      []string{policy.ManagedUserAccountPoolVarName},
		Fixture:      fixture.FakeDMS,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.WallpaperGooglePhotosIntegrationEnabled{},
				pci.VerifiedFunctionalityUI),
		},
	})
}

// WallpaperGooglePhotosIntegrationEnabled verifies that the
// WallpaperGooglePhotosIntegrationEnabled policy disables the three network
// requests sent to Google Photos from the Personalization Hub. It also
// verifies the corresponding UX, so that the collection option is disabled
// within the wallpaper collection app subpage.
func WallpaperGooglePhotosIntegrationEnabled(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	gaiaCreds, err := credconfig.PickRandomCreds(
		dma.CredsFromPool(policy.ManagedUserAccountPoolVarName))
	if err != nil {
		s.Fatal("Failed to parse managed user cred: ", err)
	}

	policyBlob := policy.NewBlob()
	policyBlob.PolicyUser = gaiaCreds.User
	if err := fdms.WritePolicyBlob(policyBlob); err != nil {
		s.Fatal("Failed to write policies to FakeDMS: ", err)
	}

	opts := []chrome.Option{
		chrome.DMSPolicy(fdms.URL),  // FakeDMS for setting policies
		chrome.GAIALogin(gaiaCreds), // Real GAIA to enable google photos
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

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctx)

	for index, param := range wallpapergooglephotos.TestCases() {
		s.Run(ctx, param.Name, func(ctx context.Context, s *testing.State) {
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
			br := cr.Browser()
			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.Name)

			// Open the net-export page and start logging.
			netExport, err := netexport.Start(ctx, cr, br, browser.TypeAsh)
			if err != nil {
				s.Fatal("Failed to start net export: ", err)
			}
			defer netExport.Cleanup(cleanupCtx)

			if err := wallpapergooglephotos.TriggerWallpaperGooglePhotosIntegration(ctx,
				networkrequestmonitor.OptionalServiceParams{
					Chrome:        cr,
					Browser:       br,
					PolicySetting: index}); err != nil {
				s.Fatal("Failure while trigger google photos integration: ", err)
			}

			hashCodes := []string{
				wallpapergooglephotos.EnabledHashCode,
				wallpapergooglephotos.AlbumsHashCode,
				wallpapergooglephotos.PhotosHashCode,
			}

			foundAnnotations, err := netExport.FindMultipleAnnotationsUntil(ctx, hashCodes,
				&testing.PollOptions{Timeout: 5 * time.Second, Interval: 1 * time.Second})
			if err != nil {
				s.Fatal("Failed to poll hashcode in log: ", err)
			}

			for _, annotationID := range hashCodes {
				if _, exists := foundAnnotations[annotationID]; exists != param.ShouldFindAnnotations {
					s.Errorf("Unexpected status of annotation = %s, got %t, want %t", annotationID, exists, param.ShouldFindAnnotations)
				}
			}
		})
	}
}
