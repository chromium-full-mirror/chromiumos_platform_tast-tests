// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         NearbyShareAllowed,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test NearbyShareAllowed policy",
		Contacts: []string{
			"dp-chromeos-eng@google.com",
			"chiav@google.com",
		},
		BugComponent: "b:1129862",
		Attr:         []string{"group:golden_tier"},
		SoftwareDeps: []string{"chrome"},
		VarDeps:      []string{"policy.managedUserAccountPool"},
		Fixture:      fixture.FakeDMS,
		Timeout:      3 * time.Minute,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.NearbyShareAllowed{}, pci.VerifiedFunctionalityUI),
		},
	})
}

// NearbyShareAllowed tests the NearbyShareAllowed policy by verifying that the
// Nearby Share OS setting shows correctly based on policy value.
//   - When enabled: A 'Set up' button should be displayed
//   - When disabled: A on/off toggle should be set to off, and be disabled
func NearbyShareAllowed(ctx context.Context, s *testing.State) {
	const (
		connectedDevicesURL      = "multidevice"
		connectedDevicesPageName = "Connected devices"
		setupButtonName          = "Set up"
		toggleName               = "Nearby Share"
	)

	// Shorten the context to make room for cleanup jobs.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

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
		chrome.GAIALogin(gaiaCreds), // Real GAIA to enable 'Connected devices'
	}

	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Test the 'enabled' and 'disabled' cases only, since these are the valid values in DPanel.
	// No test for 'unset' since this is not a valid value for this policy.
	for _, param := range []struct {
		name             string
		shouldBeDisabled bool
		policy           *policy.NearbyShareAllowed
	}{
		{
			name:             "enabled",
			shouldBeDisabled: false,
			policy:           &policy.NearbyShareAllowed{Val: true},
		},
		{
			name:             "disabled",
			shouldBeDisabled: true,
			policy:           &policy.NearbyShareAllowed{Val: false},
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

			// Open 'Connected devices' page in OS Settings.
			ui := uiauto.New(tconn)
			settings, err := ossettings.LaunchAtPageURL(ctx, tconn, cr, connectedDevicesURL,
				ui.WaitUntilExists(nodewith.Name(connectedDevicesPageName).First()))
			if err != nil {
				s.Fatal("Failed to launch Connected devices OS Settings page: ", err)
			}

			if param.shouldBeDisabled {
				// Verify toggle is set to disabled.
				if isEnabled, err := settings.IsToggleOptionEnabled(ctx, cr, toggleName); err != nil {
					s.Fatal("Failed to get current toggle value: ", err)
				} else if isEnabled {
					s.Fatal("Toggle is enabled when it should be disabled: ", err)
				}

				// Verify toggle is restricted (cannot be changed).
				nearbyShareToggle := nodewith.Name(toggleName).Role(role.ToggleButton)
				info, err := ui.Info(ctx, nearbyShareToggle)
				if err != nil {
					s.Fatal("Failed to get info about nearby share toggle: ", err)
				}
				if info.Restriction != restriction.Disabled {
					s.Fatal("Nearby share toggle is not restricted: ", err)
				}
			} else {
				// Verify 'Set up' button is displayed.
				nearbyShareSetupButton := nodewith.Name(setupButtonName).Role(role.Button)
				if err := ui.WaitUntilExists(nearbyShareSetupButton)(ctx); err != nil {
					s.Fatal("Nearby share 'Set up' button missing: ", err)
				}
			}
		})
	}
}
