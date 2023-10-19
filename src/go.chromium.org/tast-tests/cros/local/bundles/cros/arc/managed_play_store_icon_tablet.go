// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ManagedPlayStoreIconTablet,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests the visibility of Play Store icon on tablet form factor w.r.t. ArcPolicy",
		Contacts:     []string{"arc-commercial@google.com", "yaohuali@google.com"},
		// ChromeOS > Software > ARC++ > Commercial
		BugComponent: "b:157100",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container", "tablet_form_factor"},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm", "tablet_form_factor"},
		}},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ArcEnabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.SyncDisabled{}, pci.VerifiedFunctionalityUI),
		},
	})
}

// ManagedPlayStoreIconTablet tests the visibility of Play Store icon w.r.t ArcEnabled policy on tablet form factor.
// On tablet only, when ARC is disabled by policy, Play Store icon still appears on shelf.
func ManagedPlayStoreIconTablet(ctx context.Context, s *testing.State) {
	// Start FakeDMS.
	fdms, err := fakedms.New(ctx, s.OutDir())
	if err != nil {
		s.Fatal("Failed to start FakeDMS: ", err)
	}
	defer fdms.Stop(ctx)

	if err := fdms.WritePolicyBlob(policy.NewBlob()); err != nil {
		s.Fatal("Failed to write policies to FakeDMS: ", err)
	}

	// Start a Chrome instance that will fetch policies from the FakeDMS.
	cr, err := chrome.New(ctx,
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fdms.URL),
		chrome.ARCSupported(),
	)
	if err != nil {
		s.Fatal("Chrome startup failed: ", err)
	}
	defer cr.Close(ctx)

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Set up keyboard.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctx)

	for _, param := range []struct {
		name        string
		wantEnabled bool               // wantEnabled is whether we want ARC enabled.
		value       *policy.ArcEnabled // value is the value of the policy.
	}{
		{
			name:        "enable",
			wantEnabled: true,
			value:       &policy.ArcEnabled{Val: true, Stat: policy.StatusSet},
		},
		{
			name:        "disable",
			wantEnabled: false,
			value:       &policy.ArcEnabled{Val: false},
		},
		{
			name:        "unset",
			wantEnabled: false,
			value:       &policy.ArcEnabled{Stat: policy.StatusUnset},
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Sync needs to be disabled for shelf icons to show. See b/303237403#comment12
			if err := policyutil.ServeAndRefresh(ctx, fdms, cr, []policy.Policy{param.value, &policy.SyncDisabled{Val: true}}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			// Look for the Play Store icon and click.
			// Polling till the icon is found or the timeout is reached.
			uia := uiauto.New(tconn)
			notFoundError := errors.New("Play Store icon is not found yet")
			err := testing.Poll(ctx, func(ctx context.Context) error {
				if found, err := uia.IsNodeFound(ctx, nodewith.Name(apps.PlayStore.Name).ClassName("ash/ShelfAppButton")); err != nil {
					if errors.Is(err, context.DeadlineExceeded) {
						return err
					}
					return testing.PollBreak(errors.Wrap(err, "failed to check Play Store icon"))
				} else if found {
					return nil
				}
				return notFoundError
			}, &testing.PollOptions{Timeout: 15 * time.Second, Interval: time.Second})

			// We expect Play icon to always appear on tablet, regardless whether ARC is enabled by policy.
			if err != nil {
				s.Fatal("Failed to confirm the Play Store icon: ", err)
			}

			// Click on Play Store icon and see what pops out.
			if err := apps.Launch(ctx, tconn, apps.PlayStore.ID); err != nil {
				s.Fatal("Failed to launch Play Store: ", err)
			}

			if param.wantEnabled {
				// ARC opt-in is expected to happen (but fail, due to the fake policy we gave).
				// This will take a long time, e.g. 30s on kakadu, thus a long timeout value.
				arcOptInUI := nodewith.Name("Google Play apps and services").Role(role.StaticText)
				if err := uia.WithTimeout(50 * time.Second).WaitUntilExists(arcOptInUI)(ctx); err != nil {
					s.Fatal("Failed to see ARC Opt-In UI: ", err)
				}
				// Reset Chrome will close the ARC opt-in window.
			} else {
				// Verify that ARC isn't available.
				if err := checkAndDismissPlayStoreUnavailablePopup(ctx, uia); err != nil {
					s.Fatal("Failed to check Play Store unavailable pop-up window: ", err)
				}
			}
		})
	}
}

func checkAndDismissPlayStoreUnavailablePopup(ctx context.Context, uia *uiauto.Context) error {
	// On tablet, a pop-up window will inform user that Play Store is not available.
	popupUI := nodewith.Name("This app requires access to the Play Store").Role(role.Window)
	if err := uia.WithTimeout(3 * time.Second).WaitUntilExists(popupUI)(ctx); err != nil {
		return errors.Wrap(err, "failed to see pop-up window")
	}

	// Click OK button to dismiss pop-up window.
	button := nodewith.Name("OK").Role(role.Button).Ancestor(popupUI)
	if err := uia.WithTimeout(3 * time.Second).LeftClick(button)(ctx); err != nil {
		return errors.Wrap(err, "failed to click OK on pop-up window")
	}

	// Ensure pop-up window is gone. Otherwise it would interfere with next iteration.
	if err := uia.WithTimeout(2 * time.Second).WaitUntilGone(button)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for pop-up window to be dismissed")
	}

	return nil
}
