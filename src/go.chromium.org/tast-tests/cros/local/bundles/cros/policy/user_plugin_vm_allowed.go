// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/policyutil"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         UserPluginVMAllowed,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test UserPluginVmAllowed policy",
		Contacts: []string{
			"dp-chromeos-eng@google.com",
			"chiav@google.com",
		},
		BugComponent: "b:1129862",
		Attr:         []string{"group:golden_tier", "group:hardware"},
		SoftwareDeps: []string{"chrome", "plugin_vm"}, // only run on devices that can use PluginVM
		Fixture:      fixture.ChromeEnrolledLoggedIn,  // device must be enrolled
		Timeout:      3 * time.Minute,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.UserPluginVmAllowed{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.PluginVmAllowed{}, pci.VerifiedValue),
			pci.SearchFlag(&policy.PluginVmUserId{}, pci.VerifiedValue),
		},
	})
}

// UserPluginVMAllowed tests UserPluginVmAllowed policy. When the policy is unset or disabled, the Parallels Desktop
// VM installer should be disabled.
func UserPluginVMAllowed(ctx context.Context, s *testing.State) {
	const (
		permissionDeniedText = "Parallels Desktop isn't allowed on this device. Contact your administrator."
		affiliationID        = "default_affiliation_id"
	)

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	for _, param := range []struct {
		name                  string
		shouldVMSetupBeDenied bool
		policy                *policy.UserPluginVmAllowed
	}{
		{
			name:                  "unset",
			shouldVMSetupBeDenied: true,
			policy:                &policy.UserPluginVmAllowed{Stat: policy.StatusUnset},
		},
		{
			name:                  "enabled",
			shouldVMSetupBeDenied: false,
			policy:                &policy.UserPluginVmAllowed{Val: true},
		},
		{
			name:                  "disabled",
			shouldVMSetupBeDenied: true,
			policy:                &policy.UserPluginVmAllowed{Val: false},
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Set user affiliation, since VM setup will be disabled if user is not affiliated. Note: This must be done with
			// its own ServeBlobAndRefresh call. Setting policies and user affiliation in one call does not work.
			policyBlob := policy.NewBlob()
			policyBlob.DeviceAffiliationIds = []string{affiliationID}
			policyBlob.UserAffiliationIds = []string{affiliationID}
			if err := policyutil.ServeBlobAndRefresh(ctx, fdms, cr, policyBlob); err != nil {
				s.Fatal("Failed to set user affiliation policies: ", err)
			}

			// Update policies.
			policies := []policy.Policy{
				param.policy,
				// Set policies that are required for VM installation to be enabled.
				&policy.PluginVmAllowed{Val: true},
				&policy.PluginVmUserId{Stat: policy.StatusSet, Val: "********"}}
			policyBlob.AddPolicies(policies)
			if err := policyutil.ServeBlobAndRefresh(ctx, fdms, cr, policyBlob); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}
			if err := policyutil.Verify(ctx, tconn, policies); err != nil {
				s.Fatal("Failed to verify updated policies: ", err)
			}

			// Open the PluginVM installer window.
			if err := tconn.Eval(ctx, `chrome.autotestPrivate.showPluginVMInstaller()`, nil); err != nil {
				s.Fatal("Failed to show plugin VM installer: ", err)
			}

			ui := uiauto.New(tconn)

			// Click 'Install' button on PluginVM installer.
			if err := clickButton(ctx, ui, "Install"); err != nil {
				s.Fatal("Failed to click 'Install' button: ", err)
			}

			// Check installer window for permission denied error message.
			permissionDeniedNode := nodewith.NameStartingWith(permissionDeniedText).Role(role.StaticText)
			permissionDenied := false
			if err := ui.WithTimeout(5 * time.Second).WaitUntilExists(permissionDeniedNode)(ctx); err != nil {
				if !nodewith.IsNodeNotFoundErr(err) {
					s.Fatal("Failed waiting for permission denied message: ", err)
				} // else do nothing since node was not found
			} else {
				permissionDenied = true
			}

			// Close PluginVM installer.
			if err := clickButton(ctx, ui, "Cancel"); err != nil {
				s.Fatal("Failed to close PluginVM installer: ", err)
			}

			if permissionDenied && !param.shouldVMSetupBeDenied {
				s.Fatal("VM setup permission denied when it should be allowed")
			}
			if !permissionDenied && param.shouldVMSetupBeDenied {
				s.Fatal("VM setup permission allowed when it should be denied")
			}
		})
	}
}

// clickButton clicks a button on the PluginVM installer window. The window sometimes ignores button clicks,
// so this helper function uses polling to retry.
func clickButton(ctx context.Context, ui *uiauto.Context, name string) error {
	window := nodewith.ClassName("PluginVmInstallerView").Role(role.Window)
	button := nodewith.Name(name).ClassName("MdTextButton").Ancestor(window)

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := uiauto.Combine(fmt.Sprintf("Click %s button", name),
			ui.WaitUntilExists(button),
			ui.DoDefault(button),
			ui.WithTimeout(1*time.Second).WaitUntilGone(button),
		)(ctx); err != nil {
			// Something failed. Return wrapped error, which will cause the poll function to retry the action.
			return errors.Wrapf(err, "failed to click %s button", name)
		}
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
		return err
	}
	return nil
}
