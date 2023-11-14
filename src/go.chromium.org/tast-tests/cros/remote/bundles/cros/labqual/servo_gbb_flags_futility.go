// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package labqual

import (
	"context"
	"time"

	common "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/checkers"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	pb "go.chromium.org/tast-tests/cros/services/cros/firmware"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type setGbbFlagsFunc func(string)

func init() {
	testing.AddTest(&testing.Test{
		Func: ServoGBBFlagsFutility,
		Desc: "Checks that device GBB flags can be set and tests whether GBB can be read/set via servo",
		Contacts: []string{
			"peep-fleet-infra-sw@google.com",
		},
		BugComponent: "b:1032353", // Chrome Operations > Fleet > Software > OS Fleet Automation
		Fixture:      fixture.NormalMode,
		Timeout:      15 * time.Minute, // Increased timeout to allow sufficient time for DUT to reboot
		Attr:         []string{"group:labqual_informational", "group:labqual_stable"},
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

func ServoGBBFlagsFutility(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}

	old, err := common.GetGBBFlagsByServo(ctx, h.ServoProxy)
	if err != nil {
		s.Fatal("Failed to read initial GBB flags: ", err)
	}
	s.Log("Initial GBB flags: ", old.Set)

	req := &pb.GBBFlagsState{Set: common.GBBToggle(old.Set, pb.GBBFlag_DEV_SCREEN_SHORT_DELAY), Clear: common.GBBToggle(old.Clear, pb.GBBFlag_DEV_SCREEN_SHORT_DELAY)}

	if _, err = common.ClearAndSetGBBFlagsByServo(ctx, h.ServoProxy, req); err != nil {
		s.Fatal("Failed initial ClearAndSetGBBFlagsByServo: ", err)
	}
	ctxForCleanup := ctx
	// 3 minutes is a ballpark estimate based on testing this on several boards.
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Minute)
	defer cancel()

	checker := checkers.New(h)
	defer func(ctx context.Context) {
		if _, err := common.ClearAndSetGBBFlagsByServo(ctx, h.ServoProxy, old); err != nil {
			s.Fatal("ClearAndSetGBBFlagsByServo to restore original values failed: ", err)
		}

		if err := h.EnsureDUTBooted(ctx); err != nil {
			s.Fatal("Failed to reconnect to DUT after reboot: ", err)
		}

		if err := checker.GBBFlagsByServo(ctx, old); err != nil {
			s.Fatal("all flags should have been restored: ", err)
		}
	}(ctxForCleanup)

	if err := h.EnsureDUTBooted(ctx); err != nil {
		s.Fatal("Failed to reconnect to DUT after reboot: ", err)
	}

	if err := checker.GBBFlagsByServo(ctx, req); err != nil {
		s.Fatal("DEV_SCREEN_SHORT_DELAY flag should have been toggled: ", err)
	}
}
