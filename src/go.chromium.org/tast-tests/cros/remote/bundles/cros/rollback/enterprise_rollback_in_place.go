// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rollback

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/remote/rollback"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         EnterpriseRollbackInPlace,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check the enterprise rollback data restore mechanism while faking a rollback on one image",
		Contacts: []string{
			// DO NOT modify these tests without approval from a test contact.
			"chromeos-commercial-remote-management@google.com",
			"mpolzer@google.com", // Test author
			"crisguerrero@chromium.org",
		},
		BugComponent: "b:1031231",
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
		},
		SoftwareDeps: []string{"reboot", "chrome"},
		ServiceDeps: []string{
			"tast.cros.hwsec.OwnershipService",
			"tast.cros.rollback.EnterpriseRollbackService",
		},
		Timeout: 10 * time.Minute,
		SearchFlags: []*testing.StringPair{{
			Key: "feature_id",
			// Configure "Roll back to target version" in Admin Console
			// policy and ensure that supported devices correctly roll
			// back while preserving networks and enrollment.
			// rollback_target=current-lts
			// COM_FOUND_CUJ13_TASK4_WF4
			Value: "screenplay-7e91d2bb-7abf-441f-a31f-72d2d49bf029",
		}, {
			Key: "feature_id",
			// Configure "Roll back to target version" in Admin Console
			// policy and ensure that supported devices correctly roll
			// back while preserving networks and enrollment.
			// rollback_target=current-ltc
			// COM_FOUND_CUJ13_TASK4_WF5
			Value: "screenplay-0623ac53-18d4-46f2-b0fa-43850d93dd37",
		}},
	})
}

// EnterpriseRollbackInPlace does not expect to use enrollment so any
// functionality that depend on the enrollment of the device should be not be
// added to this test. Refer to EnterpriseRollbackEnrolled instead.
func EnterpriseRollbackInPlace(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Minute)
	defer cancel()
	defer func(ctx context.Context) {
		if err := rollback.ClearRollbackAndSystemData(ctx, s.DUT(), s.RPCHint()); err != nil {
			s.Error("Failed to clean rollback data after test: ", err)
		}
	}(cleanupCtx)

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

	// Ineffective reset is ok here as the device steps through oobe automatically
	s.Log("Simulating powerwash and rebooting the DUT to fake rollback")
	if err := rollback.SimulatePowerwashAndReboot(ctx, s.DUT()); err != nil && !errors.Is(err, hwsec.ErrIneffectiveReset) {
		s.Fatal("Failed to simulate powerwash and reboot to fake an enterprise rollback: ", err)
	}

	if err := rollback.VerifyRollbackData(ctx, s.DUT(), s.RPCHint(), networksInfo, sensitive); err != nil {
		s.Fatal("Failed to verify rollback: ", err)
	}
}
