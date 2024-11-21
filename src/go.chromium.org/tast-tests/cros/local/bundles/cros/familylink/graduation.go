// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package familylink

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/family"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/familylink"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: Graduation,
		Desc: "Checks that Graduation app can be opened when policy is enabled",
		Contacts: []string{
			"cros-families-eng+test@google.com",
			"chromeos-consumer-engprod@google.com",
		},
		// ChromeOS > Software > Family > Edusumer
		BugComponent: "b:1631240",
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:hw_agnostic",
		},
		SoftwareDeps: []string{"chrome", "gaia"},
		Timeout:      2 * time.Minute,
		Fixture:      "eduWithTakeoutLogin",
		VarDeps: []string{
			family.EduAccountVarName,
		},
	})
}

func Graduation(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn := s.FixtValue().(familylink.HasTestConn).TestConn()
	ui := uiauto.New(tconn)

	if cr == nil {
		s.Fatal("Failed to start Chrome")
	}
	if tconn == nil {
		s.Fatal("Failed to create test API connection")
	}
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	graduationPolicy := familylink.CreateGraduationPolicy()
	policies := []policy.Policy{
		graduationPolicy,
	}
	pb := policy.NewBlob()
	pb.PolicyUser = s.FixtValue().(familylink.HasPolicyUser).PolicyUser()
	pb.AddPolicies(policies)
	if err := policyutil.ServeBlobAndRefresh(ctx, fdms, cr, pb); err != nil {
		s.Fatal("Failed to serve policies: ", err)
	}

	s.Log("Verifying policies were delivered to device")
	if err := policyutil.Verify(ctx, tconn, policies); err != nil {
		s.Fatal("Failed to verify policies: ", err)
	}

	appItemOnShelf := nodewith.NameContaining("Content Transfer").ClassName(ash.ShelfAppButtonClassName)
	if err := ui.WaitUntilExists(appItemOnShelf)(ctx); err != nil {
		s.Fatal("Graduation app did not appear on shelf: ", err)
	}
	if err := ui.LeftClick(appItemOnShelf)(ctx); err != nil {
		s.Fatal("Failed to click on the Graduation app on shelf: ", err)
	}

	getStarted := nodewith.NameContaining("Get Started").Role(role.Button)
	if err := ui.WaitUntilExists(getStarted)(ctx); err != nil {
		s.Fatal("Could not find the opened Graduation app: ", err)
	}
	if err := ui.LeftClick(getStarted)(ctx); err != nil {
		s.Fatal("Failed to click on the Get Started button: ", err)
	}

	transferContent := nodewith.Name("Transfer your content").Role(role.StaticText).First()
	if err := ui.WaitUntilExists(transferContent)(ctx); err != nil {
		s.Fatal("Takeout site is not loaded: ", err)
	}
}
