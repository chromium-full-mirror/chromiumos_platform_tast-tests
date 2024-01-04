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
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DevicePolicyPropagationE2E,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "E2E test for device policy propagation",
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
		VarDeps: []string{tape.ServiceAccountVar, "ui.signinProfileTestExtensionManifestKey"},
		Timeout: 20 * time.Minute,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceAutoUpdateDisabled{}, pci.Served),
			{
				Key: "feature_id",
				// As an admin, I want to ensure that right policies are
				// propagated to my managed devices, so I verify that
				// the device has received updated policies in
				// under 10 minutes.
				// TODO(b/308932300): Integrate crosbolt.
				Value: "screenplay-b0bb6119-2d9c-4e04-810e-bb4b83b96889",
			},
		},
		Fixture: fixture.TAPEEnrolled,
	})
}

func DevicePolicyPropagationE2E(ctx context.Context, s *testing.State) {
	fixtData := fixture.TAPEAccountData{}
	if err := s.FixtFillValue(&fixtData); err != nil {
		s.Fatal("Failed to deserialize remote fixture data: ", err)
	}

	const policyPropagationTimeout = 10 * time.Minute

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	cr, err := chrome.New(ctx,
		chrome.NoLogin(),
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
		chrome.KeepEnrollment(),
	)
	if err != nil {
		s.Fatal("Failed to start chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create a TestConn: ", err)
	}

	tapeClient, err := tape.NewClient(ctx, []byte(s.RequiredVar(tape.ServiceAccountVar)))
	if err != nil {
		s.Fatal("Failed to create a tape client: ", err)
	}

	s.Run(ctx, "initial", func(context.Context, *testing.State) {
		tapePolicies := &tape.AutoUpdateSettingsDevices{
			UpdateDisabled: true,
		}
		if err := tapeClient.SetPolicy(ctx, tapePolicies, []string{"updateDisabled"} /*updateMask*/, nil /*additionalTargetKeys*/, fixtData.RequestID); err != nil {
			s.Fatal("Failed to set initial policy: ", err)
		}

		expectedPolicies := []policy.Policy{
			&policy.DeviceAutoUpdateDisabled{Stat: policy.StatusSet, Val: true},
		}

		ctx, cancel := context.WithTimeout(ctx, policyPropagationTimeout)
		defer cancel()
		if err := policyutil.WaitForPolicies(ctx, tconn, expectedPolicies); err != nil {
			s.Error("Failed to verify initial policy: ", err)
		}
	})

	s.Run(ctx, "update", func(context.Context, *testing.State) {
		pv := perf.NewValues()
		defer func() {
			if err := pv.Save(s.OutDir()); err != nil {
				s.Error("Failed saving perf data: ", err)
			}
		}()

		tapePolicies := &tape.AutoUpdateSettingsDevices{
			UpdateDisabled: false,
			// Set required fields for API.
			AutoUpdateAllowedConnectionType: tape.AUTOUPDATECONNECTIONTYPEENUM_AUTO_UPDATE_CONNECTION_TYPE_ENUM_ALL_CONNECTIONS,
			DeviceRollbackToTargetVersion:   1, // Roll back OS disabled.
			AutoUpdateRolloutPlan: tape.AutoUpdateRolloutPlan{
				Plan:    tape.ROLLOUTPLAN_SCATTER_UPDATES,
				Scatter: tape.SCATTERFACTOR_ONE_DAY,
			},
			ReleaseChannelWithLts: tape.RELEASECHANNELWITHLTSENUM_RELEASE_CHANNEL_WITH_LTS_ENUM_STABLE_CHANNEL,
			AutoUpdateTargetVersionLts: tape.AutoUpdateTargetVersionLts{
				SelectedVersion: tape.SelectedVersion{
					DisplayName: "No restriction",
				},
			},
		}
		perf.RecordExecutionTime(
			pv,
			"set_policy_time",
			func() {
				if err := tapeClient.SetPolicy(ctx, tapePolicies, []string{} /*updateMask*/, nil /*additionalTargetKeys*/, fixtData.RequestID); err != nil {
					s.Fatal("Failed to set updated policy: ", err)
				}
			},
		)

		ctx, cancel := context.WithTimeout(ctx, policyPropagationTimeout)
		defer cancel()
		expectedPolicies := []policy.Policy{
			&policy.DeviceAutoUpdateDisabled{Stat: policy.StatusSet, Val: false},
		}
		perf.RecordExecutionTime(
			pv,
			"propagation_time",
			func() {
				if err := policyutil.WaitForPolicies(ctx, tconn, expectedPolicies); err != nil {
					s.Fatal("Failed to verify updated policy: ", err)
				}
			},
		)
	})
}
