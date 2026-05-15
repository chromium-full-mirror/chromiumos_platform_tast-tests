// Copyright 2021 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/familylink"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     UnicornDefaultApps,
		Desc:     "Verifies the Default arc apps for Unicorn Account",
		Contacts: []string{"arc-commercial@google.com", "cros-arc-te@google.com", "jinrongwu@google.com", "mattlui@google.com"},
		// ChromeOS > Software > ARC++ > Commercial > Tast Tests
		BugComponent: "b:1487630",
		Attr:         []string{"group:mainline", "group:arc-functional"},
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 3*time.Minute,
		SoftwareDeps: []string{"chrome", "gaia"},
		Params: []testing.Param{
			{
				Name:              "vm",
				ExtraSoftwareDeps: []string{"android_vm", "no_qemu"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name:              "betty_vm",
				ExtraSoftwareDeps: []string{"android_vm", "qemu"},
				ExtraAttr:         []string{"informational", "group:hw_agnostic"},
			}},
		Fixture: "familyLinkUnicornArcPolicyLogin",
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ArcEnabled{}, pci.VerifiedFunctionalityOS),
		},
	})
}

func UnicornDefaultApps(ctx context.Context, s *testing.State) {

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn := s.FixtValue().(familylink.HasTestConn).TestConn()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	arcEnabledPolicy := &policy.ArcEnabled{Val: true}

	policies := []policy.Policy{arcEnabledPolicy}

	pb := policy.NewBlob()
	pb.AddPolicies(policies)
	if err := policyutil.ServeBlobAndRefresh(ctx, fdms, cr, pb); err != nil {
		s.Fatal("Failed to serve policies: ", err)
	}

	// Ensure chrome://policy shows correct ArcEnabled value.
	if err := policyutil.Verify(ctx, tconn, []policy.Policy{&policy.ArcEnabled{Val: true}}); err != nil {
		s.Fatal("Failed to verify ArcEnabled: ", err)
	}

	if err := waitForAppUninstall(ctx, tconn, apps.PlayBooks.ID, 5*time.Minute); err != nil {
		s.Fatal("PlayBooks is installed even after wait: ", err)
	}

	// List for ARC++ default apps not to be present on Child Account.
	disallowedApps := []apps.App{
		apps.PlayBooks,
		apps.PlayGames,
		apps.GoogleTV,
		apps.Photos,
		apps.Clock,
	}

	version, err := arc.SDKVersion()
	if err != nil {
		s.Fatal("Failed to get ARC version: ", err)
	}

	if version < arc.SDKT {
		// Contacts is a required app on T+ so should not be removed.
		disallowedApps = append(disallowedApps, apps.Contacts)
	}

	for _, app := range disallowedApps {
		installed, err := ash.ChromeAppInstalled(ctx, tconn, app.ID)
		if err != nil {
			s.Fatal("Failed to check ChromeAppInstalled: ", err)
		} else if installed {
			s.Errorf("App %s (%s) is installed on child account", app.Name, app.ID)
		}
	}

}

// waitForAppUninstall waits for the app specified by appID to be uninstalled.
func waitForAppUninstall(ctx context.Context, tconn *chrome.TestConn, appID string, timeout time.Duration) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		if installed, err := ash.ChromeAppInstalled(ctx, tconn, appID); err != nil {
			return testing.PollBreak(err)
		} else if installed {
			return errors.New("failed to wait for installed app by id: " + appID)
		}
		return nil
	}, &testing.PollOptions{Timeout: timeout})
}
