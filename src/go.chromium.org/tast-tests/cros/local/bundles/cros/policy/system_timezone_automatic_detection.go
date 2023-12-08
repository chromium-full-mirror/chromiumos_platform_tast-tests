// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/checked"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SystemTimezoneAutomaticDetection,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check of SystemTimezoneAutomaticDetection policy by checking the settings page",
		Contacts: []string{
			"cros-engprod-muc@google.com",
			"vsavu@google.com", // Test author
		},
		BugComponent: "b:1263917",
		Attr:         []string{"group:golden_tier", "group:hw_agnostic"},
		SoftwareDeps: []string{"reboot", "chrome"},
		Fixture:      fixture.ChromeEnrolledLoggedIn,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.SystemTimezoneAutomaticDetection{}, pci.VerifiedFunctionalityUI),
		},
	})
}

func SystemTimezoneAutomaticDetection(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	for _, param := range []struct {
		// name is the subtest name.
		name string
		// value is the policy value.
		value *policy.SystemTimezoneAutomaticDetection
		// wantRestriction indicates whether the setting can be changed by the user.
		wantRestriction restriction.Restriction
		// selectedOption is the selected state for timezone detection.
		selectedOption string
		// selectedDetection is the state for timezone detection.
		selectedDetection string
	}{
		{
			name:              "all",
			value:             &policy.SystemTimezoneAutomaticDetection{Val: 4},
			wantRestriction:   restriction.Disabled,
			selectedOption:    "Set automatically",
			selectedDetection: "Use Wi-Fi or mobile networks to determine location",
		},
		{
			name:              "wifi",
			value:             &policy.SystemTimezoneAutomaticDetection{Val: 3},
			wantRestriction:   restriction.Disabled,
			selectedOption:    "Set automatically",
			selectedDetection: "Use only Wi-Fi to determine location",
		},
		{
			name:              "ip",
			value:             &policy.SystemTimezoneAutomaticDetection{Val: 2},
			wantRestriction:   restriction.Disabled,
			selectedOption:    "Set automatically",
			selectedDetection: "Use your IP address to determine location (default)",
		},
		{
			name:              "never",
			value:             &policy.SystemTimezoneAutomaticDetection{Val: 1},
			wantRestriction:   restriction.Disabled,
			selectedOption:    "Choose from list",
			selectedDetection: "Automatic time zone detection is disabled",
		},
		{
			name:              "user",
			value:             &policy.SystemTimezoneAutomaticDetection{Val: 0},
			wantRestriction:   restriction.None,
			selectedOption:    "Set automatically",
			selectedDetection: "Use your IP address to determine location (default)",
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			// Retry to make sure they are applied. This is just an experiment for applying device policies.
			if err := testing.Poll(ctx, func(ctx context.Context) error {
				return policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.value})
			}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			// Open the time zone settings page.
			if err := policyutil.OSSettingsPage(ctx, cr, "dateTime/timeZone").
				SelectNode(ctx, nodewith.
					Role(role.RadioButton).
					Name(param.selectedOption)).
				Checked(checked.True).
				Restriction(param.wantRestriction).
				Verify(); err != nil {
				s.Error("Unexpected OS settings state: ", err)
			}

			// Check the currently selected detection mode.
			uia := uiauto.New(tconn)
			text, err := uia.Info(ctx, nodewith.Name("Time zone detection method"))
			if err != nil {
				s.Fatal("Could not find current detection mode: ", err)
			}
			if text.Value != param.selectedDetection {
				s.Errorf("Invalid detection mode: got %q; want %q", text.Value, param.selectedDetection)
			}
		})
	}
}
