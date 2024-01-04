// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         UserPolicyPropagationE2E,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "E2E test for user policy propagation",
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"vsavu@google.com",      // Test author
			"artyomchen@google.com", // Original remote test author

		},
		BugComponent: "b:1111617", // ChromeOS > Software > Commercial (Enterprise) > Remote Management > Policy Stack
		SoftwareDeps: []string{"chrome"},
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
		},
		VarDeps: []string{tape.ServiceAccountVar},
		Timeout: 20 * time.Minute,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.AllowDinosaurEasterEgg{}, pci.Served),
			{
				Key: "feature_id",
				// As an admin, I want to ensure that right policies are
				// propagated to my managed users, so I verify that
				// the user has received updated policies
				// in under 10 minutes.
				// TODO(b/308932300): Integrate crosbolt.
				Value: "screenplay-8283d4fd-bb4f-42fb-a805-ae309cee77a5",
			},
		},
		Params: []testing.Param{{
			Name:    "consumer",
			Fixture: fixture.ChromeTAPELoggedIn,
		}, {
			Name:    "enrolled",
			Fixture: fixture.ChromeTAPEEnrolledLoggedIn,
		}},
	})
}

func UserPolicyPropagationE2E(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fixtData := s.FixtValue().(*fixtures.TAPEChromeFixtData)

	const policyPropagationTimeout = 10 * time.Minute

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create a TestAPIConn: ", err)
	}

	tapeClient, err := tape.NewClient(ctx, []byte(s.RequiredVar(tape.ServiceAccountVar)))
	if err != nil {
		s.Fatal("Failed to create a tape client: ", err)
	}

	initialPolicyFailedToBeVerified := false
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
			// This test tolerates occasional propagation timeouts due to DMServer's
			// 24h SLO for invalidations. We rely on Crosbolt metrics and Perfmon
			// alerts instead.
			s.Log("Failed to verify initial policy: ", err)
			initialPolicyFailedToBeVerified = true
		}
	})
	if initialPolicyFailedToBeVerified {
		s.Log("Skipping the rest of the test")
		return
	}

	s.Run(ctx, "update", func(context.Context, *testing.State) {
		pv := perf.NewValues()
		defer func() {
			if err := pv.Save(s.OutDir()); err != nil {
				s.Error("Failed saving perf data: ", err)
			}
		}()

		tapePolicies := &tape.AllowDinosaurEasterEggUsers{
			AllowDinosaurEasterEgg: tape.NULLABLEBOOLEAN_TRUE,
		}
		if err := perf.RecordExecutionTime(
			pv,
			"set_policy_time",
			func() error {
				return tapeClient.SetPolicy(ctx, tapePolicies, []string{} /*updateMask*/, nil /*additionalTargetKeys*/, fixtData.RequestID)
			},
		); err != nil {
			s.Fatal("Failed to set updated policy: ", err)
		}

		ctx, cancel := context.WithTimeout(ctx, policyPropagationTimeout)
		defer cancel()
		expectedPolicies := []policy.Policy{
			&policy.AllowDinosaurEasterEgg{Stat: policy.StatusSet, Val: true},
		}
		perf.RecordExecutionTime(
			pv,
			"propagation_time",
			func() error {
				err := policyutil.WaitForPolicies(ctx, tconn, expectedPolicies)
				if err != nil {
					// This test tolerates occasional propagation timeouts due to
					// DMServer's 24h SLO for invalidations. We rely on Crosbolt metrics
					// and Perfmon alerts instead.
					s.Log("Failed to verify updated policy: ", err)
				}
				return err
			},
		)
	})
}
