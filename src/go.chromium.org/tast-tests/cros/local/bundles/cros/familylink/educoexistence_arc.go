// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package familylink is used for writing Family Link tests.
package familylink

import (
	"context"
	"time"

	androidui "go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/familylink"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         EducoexistenceArc,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks ARC behavior for account added via in-session EDU Coexistence flow",
		Contacts: []string{
			"cros-families-eng+test@google.com",
			"chromeos-sw-engprod@google.com",
			"anastasiian@chromium.org",
			"amberhaynes@chromium.org",
		},
		// ChromeOS > Software > Family > Parental controls
		BugComponent: "b:1090157",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_p"},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
		Timeout: chrome.GAIALoginTimeout + 5*time.Minute,
		VarDeps: []string{"arc.parentUser", "arc.parentPassword", "family.eduEmail", "family.eduPassword"},
		Fixture: "familyLinkUnicornArcPolicyLogin",
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ArcEnabled{}, pci.VerifiedFunctionalityUI),
		},
	})
}

func EducoexistenceArc(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn := s.FixtValue().(familylink.HasTestConn).TestConn()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	parentUser := s.RequiredVar("arc.parentUser")
	parentPass := s.RequiredVar("arc.parentPassword")
	eduUser := s.RequiredVar("family.eduEmail")
	eduPass := s.RequiredVar("family.eduPassword")

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

	// Setup ARC.
	a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to connect to ARC: ", err)
	}
	defer a.Close(ctx)
	defer a.DumpUIHierarchyOnError(ctx, s.OutDir(), s.HasError)

	d, err := a.NewUIDevice(ctx)
	if err != nil {
		s.Fatal("Failed initializing UI Automator: ", err)
	}
	defer d.Close(ctx)

	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	s.Log("Launching the in-session Edu Coexistence flow")
	if err := familylink.AddEduSecondaryAccount(ctx, cr, tconn, parentUser, parentPass,
		eduUser, eduPass, true /*verifyEduSecondaryAddSuccess*/); err != nil {
		s.Fatal("Failed to complete the in-session Edu Coexistence flow: ", err)
	}

	// Open account settings in Play Store.
	if err := arc.OpenPlayStoreAccountSettings(ctx, d, tconn); err != nil {
		s.Fatal("Failed to Switch Account: ", err)
	}

	// User should not be able to switch to EDU account in Play Store because it's not shown in the list.
	accountNameButton := d.Object(androidui.ClassName("android.widget.TextView"), androidui.Text(eduUser))
	if err := accountNameButton.WaitUntilGone(ctx, 30*time.Second); err != nil {
		s.Fatal("Failed to make sure that EDU account is not shown in Play Store: ", err)
	}
}
