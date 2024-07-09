// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package familylink

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/familylink"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PolicyLogin,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks if Unicorn login with policy setup is working",
		Contacts: []string{
			"cros-families-eng+test@google.com",
			"chromeos-consumer-engprod@google.com",
		},
		// ChromeOS > Software > Family > Parental controls
		BugComponent: "b:1090157",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "gaia"},
		Timeout:      5 * time.Minute,
		Fixture:      "familyLinkUnicornPolicyLogin",
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ForceGoogleSafeSearch{}, pci.VerifiedValue),
		},
	})
}

func PolicyLogin(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn := s.FixtValue().(familylink.HasTestConn).TestConn()

	// The ForceGoogleSafeSearch policy is arbitrarily chosen just to illustrate
	// that setting policies works for Family Link users.
	policies := []policy.Policy{
		&policy.ForceGoogleSafeSearch{Val: true},
	}

	pb := policy.NewBlob()
	pb.PolicyUser = s.FixtValue().(familylink.HasPolicyUser).PolicyUser()
	pb.AddPolicies(policies)

	if err := policyutil.ServeBlobAndRefresh(ctx, fdms, cr, pb); err != nil {
		s.Fatal("Failed to serve policies: ", err)
	}

	s.Log("Verifying policies delivered to device")
	if err := policyutil.Verify(ctx, tconn, policies); err != nil {
		s.Fatal("Failed to verify policies: ", err)
	}
}
