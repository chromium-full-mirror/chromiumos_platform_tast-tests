// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/annotations"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/wallpapergooglephotos"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/input"
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
		BugComponent: "b:4617222",
		Attr: []string{
			"group:golden_tier",
			"group:hardware"},
		SoftwareDeps: []string{"chrome"},
		VarDeps:      []string{"policy.managedUserAccountPool"},
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
		s.RequiredVar("policy.managedUserAccountPool"))
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
			if err := annotations.StartLogging(ctx, cr, br, false); err != nil {
				s.Fatal("Failed to start logging: ", err)
			}

			if err := wallpapergooglephotos.TriggerWallpaperGooglePhotosIntegration(ctx, nil, br, nil, tconn, index); err != nil {
				s.Fatal("Failure while trigger google photos integration: ", err)
			}

			foundAnnotations, err := allAnnotationsFound(ctx, cr)
			if err != nil {
				// Log the error but proceed so the annotation check conditions are hit.
				// This is not an error condition for the disabled test.
				s.Log("Failed while looking for annotations: ", err)
			}

			// Stop logging.
			if err := annotations.StopLogging(ctx, cr, br); err != nil {
				s.Fatal("Failed to stop logging and check logs: ", err)
			}

			if param.ShouldFindAnnotations {
				for id, found := range foundAnnotations {
					if found == false {
						s.Fatalf("Annotation with ID %s expected and not found", id)
					}
				}
			} else {
				for id, found := range foundAnnotations {
					if found == true {
						s.Fatalf("Annotation with ID %s found when not expected", id)
					}
				}
			}
		})
	}
}

func allAnnotationsFound(ctx context.Context, cr *chrome.Chrome) (foundAnnotations map[string]bool, err error) {
	annotationFoundMap := map[string]bool{
		wallpapergooglephotos.EnabledHashCode: false,
		wallpapergooglephotos.AlbumsHashCode:  false,
		wallpapergooglephotos.PhotosHashCode:  false}
	err = testing.Poll(ctx, func(ctx context.Context) error {
		allFound := true
		for annotationID, alreadyFound := range annotationFoundMap {
			if !alreadyFound {
				annotationFoundMap[annotationID], err = annotations.CheckLogs(ctx, cr, annotationID)
				if err != nil {
					return errors.Wrap(err, "failed reading logs")
				}
			}
			allFound = allFound && annotationFoundMap[annotationID]
		}

		if allFound {
			return nil
		}
		return errors.New("All annotations have not been found yet")
	}, &testing.PollOptions{Timeout: 5 * time.Second, Interval: 1 * time.Second})

	return annotationFoundMap, err
}
