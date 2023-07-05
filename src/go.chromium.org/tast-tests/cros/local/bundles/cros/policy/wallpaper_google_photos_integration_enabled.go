// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"regexp"
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
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/personalization"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/wallpaper"
	"go.chromium.org/tast-tests/cros/local/wallpaper/constants"
)

const (
	wallpaperGooglePhotosEnabledAnnotationID = "50590711"
	wallpaperGooglePhotosAlbumsAnnotationID  = "81192642"
	wallpaperGooglePhotosPhotosAnnotationID  = "93311068"
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

	for _, param := range []struct {
		name                                  string
		shouldGooglePhotosCollectionBeEnabled bool
		shouldFindGooglePhotosAnnotations     bool
		policy                                *policy.WallpaperGooglePhotosIntegrationEnabled
	}{
		{
			name:                                  "unset",
			shouldGooglePhotosCollectionBeEnabled: true,
			shouldFindGooglePhotosAnnotations:     true,
			policy:                                &policy.WallpaperGooglePhotosIntegrationEnabled{Stat: policy.StatusUnset},
		},
		{
			name:                                  "enabled",
			shouldGooglePhotosCollectionBeEnabled: true,
			shouldFindGooglePhotosAnnotations:     true,
			policy:                                &policy.WallpaperGooglePhotosIntegrationEnabled{Val: true},
		},
		{
			name:                                  "disabled",
			shouldGooglePhotosCollectionBeEnabled: false,
			shouldFindGooglePhotosAnnotations:     false,
			policy:                                &policy.WallpaperGooglePhotosIntegrationEnabled{Val: false},
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
			br := cr.Browser()
			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			// Open the net-export page and start logging.
			if err := annotations.StartLogging(ctx, cr, br, false); err != nil {
				s.Fatal("Failed to start logging: ", err)
			}

			windows, err := ash.GetAllWindows(ctx, tconn)
			if err != nil {
				s.Fatal("Failed to get windows: ", err)
			}

			// Verify there is only one window.
			if wsCount := len(windows); wsCount != 1 {
				s.Fatal("Expected 1 window; found ", wsCount)
			}

			// Minimize the browser window to allow the OpenPersonalizationHub
			// method to right-click the desktop.
			wID := windows[0].ID
			if _, err := ash.SetWindowState(ctx, tconn, wID, ash.WMEventMinimize, false /* waitForStateChange */); err != nil {
				s.Fatal("Failed to minimize browser window: ", err)
			}

			ui := uiauto.New(tconn)
			if err := uiauto.Combine("open wallpaper page of personalization hub",
				personalization.OpenPersonalizationHub(ui),
				personalization.OpenWallpaperSubpage(ui),
			)(ctx); err != nil {
				s.Fatal("Failed to open wallpaper subpage of personalization hub: ", err)
			}

			// Loaded collections have names like name=20 Images.
			loadedCollections := nodewith.NameRegex(regexp.MustCompile(`.*\d+\s[iI]mages`)).First()
			googlePhotosCollection := loadedCollections.NameStartingWith(constants.GooglePhotosWallpaperCollection)
			if err := ui.WaitUntilExists(googlePhotosCollection)(ctx); err != nil {
				s.Fatal("Failed to wait for Google Photos wallpaper collection: ", err)
			}

			googlePhotosLink := nodewith.Name("Google Photos").Role(role.ListBoxOption)
			info, err := ui.Info(ctx, googlePhotosLink)
			if err != nil {
				s.Fatal("Failed to get google photos collection link info: ", err)
			}

			isGooglePhotosCollectionEnabled := true
			if info.HTMLAttributes["aria-disabled"] == "true" {
				isGooglePhotosCollectionEnabled = false
			}

			if err := wallpaper.SelectCollection(ui, constants.GooglePhotosWallpaperCollection)(ctx); err != nil {
				s.Fatal("Failed to select google photos item")
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

			if param.shouldGooglePhotosCollectionBeEnabled != isGooglePhotosCollectionEnabled {
				s.Fatalf("Unexpected Google Photos collection enabled state: expected %t got %t", param.shouldGooglePhotosCollectionBeEnabled, isGooglePhotosCollectionEnabled)
			}

			if param.shouldFindGooglePhotosAnnotations {
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
		wallpaperGooglePhotosEnabledAnnotationID: false,
		wallpaperGooglePhotosAlbumsAnnotationID:  false,
		wallpaperGooglePhotosPhotosAnnotationID:  false}
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
