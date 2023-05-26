// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"chromiumos/tast/local/apps"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/input"
	"chromiumos/tast/local/policyutil"
	"chromiumos/tast/local/policyutil/fixtures"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ManagedPlayStoreIcon,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Ensures ARC policy controls the visibility of Play Store icon",
		Contacts:     []string{"arc-commercial@google.com", "yaohuali@google.com"},
		// ChromeOS > Software > ARC++ > Commercial
		BugComponent: "b:157100",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		Timeout:      4 * time.Minute,
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container", "no_tablet_form_factor"},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm", "no_tablet_form_factor"},
		}},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ArcEnabled{}, pci.VerifiedFunctionalityUI),
		},
	})
}

// ManagedPlayStoreIcon verifies that the ArcEnabled policy shows/hides the Play Store icon.
func ManagedPlayStoreIcon(ctx context.Context, s *testing.State) {
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
		chrome.DeferLogin(),
	)
	if err != nil {
		s.Fatal("Chrome startup failed: ", err)
	}
	defer cr.Close(ctx)

	if err := cr.ContinueLogin(ctx); err != nil {
		s.Fatal("Chrome login failed: ", err)
	}

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
			value:       &policy.ArcEnabled{Val: true},
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

			// Update policies.
			if err := policyutil.ServeAndRefresh(ctx, fdms, cr, []policy.Policy{param.value}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			// Look for the Play Store icon.
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

			if err != nil && !errors.Is(err, notFoundError) {
				s.Fatal("Failed to confirm the status of the Play Store icon: ", err)
			}

			if enabled := err == nil; enabled != param.wantEnabled {
				s.Errorf("Unexpected Play Store icon presence; got %t, want %t", enabled, param.wantEnabled)
			}
		})
	}
}
