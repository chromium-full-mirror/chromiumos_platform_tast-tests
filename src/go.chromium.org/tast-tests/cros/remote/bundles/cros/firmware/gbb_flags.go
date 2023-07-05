// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	common "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/checkers"
	pb "go.chromium.org/tast-tests/cros/services/cros/firmware"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    GBBFlags,
		Desc:    "Verifies GBB flags state can be obtained and manipulated on the DUT",
		Timeout: 8 * time.Minute,
		Contacts: []string{
			"chromeos-faft@google.com",
			"aluo@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		ServiceDeps:  []string{"tast.cros.firmware.BiosService"},
		Attr:         []string{"group:firmware", "group:labqual_informational", "firmware_smoke"},
		SoftwareDeps: []string{"flashrom"},
	})
}

func GBBFlags(ctx context.Context, s *testing.State) {
	h := firmware.NewHelper(s.DUT(), s.RPCHint(), "", "", "", "", "", "")
	defer func() {
		if err := h.Close(ctx); err != nil {
			s.Fatal("Closing helper: ", err)
		}
	}()

	old, err := common.GetGBBFlags(ctx, s.DUT())
	if err != nil {
		s.Fatal("initial GetGBBFlags failed: ", err)
	}
	s.Log("Current GBB flags: ", old.Set)

	req := &pb.GBBFlagsState{Set: common.GBBToggle(old.Set, pb.GBBFlag_DEV_SCREEN_SHORT_DELAY), Clear: common.GBBToggle(old.Clear, pb.GBBFlag_DEV_SCREEN_SHORT_DELAY)}

	if _, err = common.ClearAndSetGBBFlags(ctx, s.DUT(), req); err != nil {
		s.Fatal("initial ClearAndSetGBBFlags failed: ", err)
	}
	ctxForCleanup := ctx
	// 150 seconds is a ballpark estimate, adjust as needed.
	ctx, cancel := ctxutil.Shorten(ctx, 150*time.Second)
	defer cancel()

	checker := checkers.New(h)
	defer func(ctx context.Context) {
		if _, err := common.ClearAndSetGBBFlags(ctx, s.DUT(), old); err != nil {
			s.Fatal("ClearAndSetGBBFlags to restore original values failed: ", err)
		}

		if err := checker.GBBFlags(ctx, old); err != nil {
			s.Fatal("all flags should have been restored: ", err)
		}
	}(ctxForCleanup)

	if err := checker.GBBFlags(ctx, req); err != nil {
		s.Fatal("DEV_SCREEN_SHORT_DELAY flag should have been toggled: ", err)
	}
}
