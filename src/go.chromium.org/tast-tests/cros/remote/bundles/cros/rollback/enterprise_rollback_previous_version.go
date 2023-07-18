// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rollback

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/remote/rollback"
	"go.chromium.org/tast-tests/cros/remote/updateutil"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type testParam struct {
	targetMilestoneFor func(int) int
}

// Tests using autoupdate.UpdateService need to be run in the lab and have tlw service running.
// See https://source.chromium.org/chromium/chromiumos/platform/tast-tests/+/main:src/go.chromium.org/tast-tests/cros/remote/bundles/cros/autoupdate/README.md

func init() {
	testing.AddTest(&testing.Test{
		Func:         EnterpriseRollbackPreviousVersion,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests the enterprise rollback feature by rolling back to a previous release",
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"mpolzer@google.com", // Test author
			"crisguerrero@chromium.org",
		},
		BugComponent: "b:1031231",
		Attr:         []string{"group:autoupdate"},
		SoftwareDeps: []string{"reboot", "chrome", "auto_update_stable"},
		ServiceDeps: []string{
			"tast.cros.nebraska.Service",
			"tast.cros.autoupdate.UpdateService",
			"tast.cros.rollback.EnterpriseRollbackService",
			"tast.cros.hwsec.OwnershipService",
		},
		Timeout: updateutil.UpdateTimeout + 12*time.Minute,
		Params: []testing.Param{{
			Name: "rollback_1_version",
			Val: testParam{
				targetMilestoneFor: func(milestone int) int {
					return milestone - 1
				},
			},
			ExtraSearchFlags: []*testing.StringPair{{
				Key: "feature_id",
				// Configure "Roll back to target version" in Admin Console
				// policy and ensure that supported devices correctly roll
				// back while preserving networks and enrollment.
				// rollback_target=n-1
				// COM_FOUND_CUJ13_TASK4_WF1
				Value: "screenplay-072c8c85-d280-472c-a99a-04cc689565ed",
			}},
		}, {
			Name: "rollback_2_versions",
			Val: testParam{
				targetMilestoneFor: func(milestone int) int {
					return milestone - 2
				},
			},
			ExtraSearchFlags: []*testing.StringPair{{
				Key: "feature_id",
				// Configure "Roll back to target version" in Admin Console
				// policy and ensure that supported devices correctly roll
				// back while preserving networks and enrollment.
				// rollback_target=n-2
				// COM_FOUND_CUJ13_TASK4_WF2
				Value: "screenplay-6f3f655c-5fa3-4d04-b513-50c049a9d762",
			}},
		}, {
			Name: "rollback_3_versions",
			Val: testParam{
				targetMilestoneFor: func(milestone int) int {
					return milestone - 3
				},
			},
			ExtraSearchFlags: []*testing.StringPair{{
				Key: "feature_id",
				// Configure "Roll back to target version" in Admin Console
				// policy and ensure that supported devices correctly roll
				// back while preserving networks and enrollment.
				// rollback_target=n-3
				// COM_FOUND_CUJ13_TASK4_WF3
				Value: "screenplay-02406ce4-6923-4d51-8a1d-cfbf5a07eb63",
			}},
		}, {
			Name: "rollback_to_next_ltc",
			Val: testParam{
				targetMilestoneFor: func(milestone int) int {
					// TODO(b:281981511) Update when we have a better way to determine the previous LTS milestone.
					return ((milestone - 1) / 6) * 6
				},
			},
			ExtraSearchFlags: []*testing.StringPair{{
				Key: "feature_id",
				// Configure "Roll back to target version" in Admin Console
				// policy and ensure that supported devices correctly roll
				// back while preserving networks and enrollment.
				// rollback_target=last LTC.
				// COM_FOUND_CUJ13_TASK4_WF5
				Value: "screenplay-0623ac53-18d4-46f2-b0fa-43850d93dd37",
			}},
		}, {
			Name: "rollback_to_next_lts",
			Val: testParam{
				targetMilestoneFor: func(milestone int) int {
					// TODO(b:281981511) Update when we have a better way to determine the previous LTS milestone.
					return ((milestone-1)/6)*6 - 6
				},
			},
			ExtraSearchFlags: []*testing.StringPair{{
				Key: "feature_id",
				// Configure "Roll back to target version" in Admin Console
				// policy and ensure that supported devices correctly roll
				// back while preserving networks and enrollment.
				// rollback_target=last LTS.
				// COM_FOUND_CUJ13_TASK4_WF5
				Value: "screenplay-0623ac53-18d4-46f2-b0fa-43850d93dd37",
			}},
		},
		},
		Fixture: fixture.Autoupdate,
	})
}

// EnterpriseRollbackPreviousVersion does not use enrollment so any
// functionality that depend on the enrollment of the device should be not be
// added to this test.
func EnterpriseRollbackPreviousVersion(ctx context.Context, s *testing.State) {
	deviceInfo, err := rollback.DUTInfo(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to get device information: ", err)
	}

	// Make sure to clear the TPM, go back to the original image, and remove all
	// remains that may be left by a faulty rollback.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 4*time.Minute)
	defer cancel()
	defer func(ctx context.Context) {
		// Powerwash may fail on the old image if there are some shared library mismatches.
		// Usually this is not a problem because we leave the system in a usable state.
		// See also b/291088732.
		if err := rollback.ClearRollbackAndSystemData(ctx, s.DUT(), s.RPCHint()); err != nil && err != rollback.ErrPowerwashFailed {
			s.Error("Failed to clean rollback data after test: ", err)
		}

		// We rely on the autoupdate fixture to restore the original image after the
		// test (b/241391509).
	}(cleanupCtx)

	// The target milestone depends on the parameter of the test.
	// Before going through any setup for the test we want to be sure that there
	// is a release for the target milestone.
	param := s.Param().(testParam)
	targetMilestone := param.targetMilestoneFor(deviceInfo.Milestone)

	// Find the latest release for milestone M.
	paygen := s.FixtValue().(updateutil.WithPaygen).Paygen()
	filtered := paygen.FilterBoard(deviceInfo.Board).FilterDeltaType("OMAHA").FilterMilestone(targetMilestone)
	latest, err := filtered.FindLatest()
	if err != nil {
		// Unreleased boards are filtered with auto_update_stable, so there should
		// be an available image.
		s.Fatalf("Failed to find the latest release for milestone %d and board %s: %v", targetMilestone, deviceInfo.Board, err)
	}

	// There is an image available for the target milestone; testing rollback.
	if err := rollback.ClearRollbackAndSystemData(ctx, s.DUT(), s.RPCHint()); err != nil {
		s.Error("Failed to clean rollback data before test: ", err)
	}

	networksInfo, err := rollback.ConfigureNetworks(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to configure networks: ", err)
	}

	sensitive, err := rollback.SaveRollbackData(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to save rollback data: ", err)
	}

	rollbackVersion := latest.ChromeOSVersion
	s.Logf("Starting update from %s to %s", deviceInfo.Version, rollbackVersion)
	if err := rollback.ToPreviousVersion(ctx, s.DUT(), s.RPCHint(), s.OutDir(), deviceInfo.Board, targetMilestone, rollbackVersion); err != nil {
		s.Fatal("Failed to rollback to previous version: ", err)
	}

	if err := rollback.CheckImageVersion(ctx, s.DUT(), s.RPCHint(), rollbackVersion, deviceInfo.Version); err != nil {
		s.Fatal("Failed to verify image after rollback: ", err)
	}

	// Check rollback data preservation.
	if err := rollback.VerifyRollbackData(ctx, s.DUT(), s.RPCHint(), networksInfo, sensitive); err != nil {
		s.Fatal("Failed to verify rollback: ", err)
	}
}
