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
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/familylink"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         UnicornDefaultApps,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies the Default arc apps for Unicorn Account",
		Contacts:     []string{"arc-commercial@google.com", "cros-arc-te@google.com", "jinrongwu@google.com"},
		// ChromeOS > Software > ARC++ > Commercial
		BugComponent: "b:157100",
		Attr:         []string{"group:mainline", "group:arc-functional", "group:hw_agnostic"},
		Timeout:      6 * time.Minute,
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container"},
			ExtraAttr:         []string{"informational"},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
			ExtraAttr:         []string{"informational"},
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
	apps := []apps.App{
		apps.PlayBooks,
		apps.PlayGames,
		apps.GoogleTV,
		apps.Photos,
		apps.Clock,
		apps.Contacts,
	}

	for _, app := range apps {
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
