// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	uiperf "go.chromium.org/tast-tests/cros/local/bundles/cros/ui/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/perfutil"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DesksChainedAnimationPerf,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Measures the smoothness of a chained desk activation animation",
		Contacts: []string{
			"chromeos-wms@google.com",
			"dandersson@google.com",
			"chromeos-sw-engprod@google.com",
		},
		//  ChromeOS > Software > Window Management > Virtual Desks
		BugComponent: "b:1238200",
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Fixture:      "chromeLoggedIn",
	})
}

func DesksChainedAnimationPerf(ctx context.Context, s *testing.State) {
	// Reserve five seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Ensure display on to record ui performance correctly.
	if err := power.TurnOnDisplay(ctx); err != nil {
		s.Fatal("Failed to turn on display: ", err)
	}

	cr := s.FixtValue().(*chrome.Chrome)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	// Add three desks for a total of four. Remove them at the end of the test.
	const numDesks = 3
	for i := 0; i < numDesks; i++ {
		if err = ash.CreateNewDesk(ctx, tconn); err != nil {
			s.Error("Failed to create a new desk: ", err)
		}
		defer ash.CleanUpDesks(cleanupCtx, tconn)
	}

	if err := perfutil.RunMultipleAndSave(ctx, s.OutDir(), cr.Browser(), uiperf.Run(s, perfutil.RunAndWaitAll(tconn, func(ctx context.Context) error {
		// Starting at desk 1, activate desk 4 by activating each adjacent desk until we reach it.
		if err = ash.ActivateAdjacentDesksToTargetIndex(ctx, tconn, 3); err != nil {
			return errors.Wrap(err, "failed to activate the fourth desk")
		}
		// Go back to desk 1.
		if err = ash.ActivateAdjacentDesksToTargetIndex(ctx, tconn, 0); err != nil {
			return errors.Wrap(err, "failed to activate the first desk")
		}
		return nil
	},
		"Ash.Desks.AnimationSmoothness.DeskActivation")),
		perfutil.StoreSmoothness,
		perfutil.RunnerOptions{IgnoreFirstRun: true, DropMinMaxValues: true},
	); err != nil {
		s.Fatal("Failed to run or save: ", err)
	}
}
