// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

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
		Attr:         []string{"group:labqual_informational"},
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
	// 150 seconds is a ballpark estimate, adjust as needed.
	ctx, cancel := ctxutil.Shorten(ctx, 150*time.Second)
	defer cancel()

	checker := checkers.New(h)
	defer func(ctx context.Context) {
		if _, err := common.ClearAndSetGBBFlagsByServo(ctx, h.ServoProxy, old); err != nil {
			s.Fatal("ClearAndSetGBBFlagsByServo to restore original values failed: ", err)
		}

		if err := checker.GBBFlagsByServo(ctx, old); err != nil {
			s.Fatal("all flags should have been restored: ", err)
		}
	}(ctxForCleanup)

	s.Log("Waiting for reboot")
	if err := h.WaitConnect(ctx); err != nil {
		s.Fatalf("Failed to connect to DUT: %s", err)
	}

	if err := checker.GBBFlagsByServo(ctx, req); err != nil {
		s.Fatal("DEV_SCREEN_SHORT_DELAY flag should have been toggled: ", err)
	}
}
