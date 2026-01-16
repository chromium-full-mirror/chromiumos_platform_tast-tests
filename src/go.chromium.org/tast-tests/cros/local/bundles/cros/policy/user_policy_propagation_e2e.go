// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: UserPolicyPropagationE2E,
		Desc: "E2E test for user policy propagation",
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"vsavu@google.com", // Test owner
		},
		BugComponent: "b:1111617", // ChromeOS > Software > Commercial (Enterprise) > Remote Management > Policy Stack
		SoftwareDeps: []string{"chrome", "gaia"},
		Fixture:      fixture.TAPEEnrolled,
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:hw_agnostic",
		},
		Timeout: 20 * time.Minute,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.AllowDinosaurEasterEgg{}, pci.Served),
		},
		Params: []testing.Param{{
			Name: "sha256_enabled",
			Val:  true,
		}, {
			Name: "sha256_disabled",
			Val:  false,
		}},
	})
}

func UserPolicyPropagationE2E(ctx context.Context, s *testing.State) {
	sha256Enabled := s.Param().(bool)
	fixtData := fixture.TAPEAccountData{}
	if err := s.FixtFillValue(&fixtData); err != nil {
		s.Fatal("Failed to deserialize remote fixture data: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	opts := []chrome.Option{
		chrome.GAIALogin(chrome.Creds{User: fixtData.Username, Pass: fixtData.Password}),
		chrome.KeepEnrollment(),
	}
	if sha256Enabled {
		opts = append(opts, chrome.EnableFeatures("PolicyFetchWithSha256"))
	} else {
		opts = append(opts, chrome.DisableFeatures("PolicyFetchWithSha256"))
	}
	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		s.Fatal("Failed to start chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create a TestAPIConn: ", err)
	}

	tapeClient, err := tape.NewClient(ctx, []byte{})
	if err != nil {
		s.Fatal("Failed to create a tape client: ", err)
	}

	const policyPropagationTimeout = 10 * time.Minute

	s.Run(ctx, "initial", func(context.Context, *testing.State) {
		tapePolicies := &tape.AllowDinosaurEasterEggUsers{
			AllowDinosaurEasterEgg: tape.NULLABLEBOOLEAN_FALSE,
		}
		if err := tapeClient.SetPolicy(ctx, tapePolicies, []string{} /*updateMask*/, nil /*additionalTargetKeys*/, fixtData.RequestID); err != nil {
			s.Fatal("Failed to set initial policy: ", err)
		}

		expectedPolicies := []policy.Policy{
			&policy.AllowDinosaurEasterEgg{Stat: policy.StatusSet, Val: false},
		}
		ctx, cancel := context.WithTimeout(ctx, policyPropagationTimeout)
		defer cancel()
		if err := policyutil.WaitForPolicies(ctx, tconn, expectedPolicies); err != nil {
			s.Fatal("Failed to verify initial policy: ", err)
		}
	})

	s.Run(ctx, "update", func(context.Context, *testing.State) {
		tapePolicies := &tape.AllowDinosaurEasterEggUsers{
			AllowDinosaurEasterEgg: tape.NULLABLEBOOLEAN_TRUE,
		}
		if err := tapeClient.SetPolicy(ctx, tapePolicies, []string{} /*updateMask*/, nil /*additionalTargetKeys*/, fixtData.RequestID); err != nil {
			s.Fatal("Failed to set updated policy: ", err)
		}

		expectedPolicies := []policy.Policy{
			&policy.AllowDinosaurEasterEgg{Stat: policy.StatusSet, Val: true},
		}
		ctx, cancel := context.WithTimeout(ctx, policyPropagationTimeout)
		defer cancel()
		if err := policyutil.WaitForPolicies(ctx, tconn, expectedPolicies); err != nil {
			s.Fatal("Failed to verify updated policy: ", err)
		}
	})
}
