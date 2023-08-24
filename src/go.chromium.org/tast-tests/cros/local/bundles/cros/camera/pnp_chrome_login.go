// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"time"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast-tests/cros/local/camera/pnp"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/power"
	powersetup "go.chromium.org/tast-tests/cros/local/power/setup"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PNPChromeLogin,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Collect power metrics when device is in idle with Chrome login",
		BugComponent: "b:167281",
		Contacts:     []string{"chromeos-camera-eng@google.com", "esker@chromium.org"},
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild", "group:camera_dependent"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      1*time.Minute + pnp.PNPTimeParams.Total + power.RecorderTimeout,
		Params: []testing.Param{{
			Name:    "ash",
			Fixture: pnp.StablePowerAshGAIA,
		}, {
			Name:    "lacros",
			Fixture: pnp.StablePowerLacrosGAIA,
		}},
	})
}

func PNPChromeLogin(ctx context.Context, s *testing.State) {
	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	browserType := s.FixtValue().(powersetup.PowerUIFixtureData).Bt
	cr := s.FixtValue().(powersetup.PowerUIFixtureData).Cr

	// Open a window with about:blank tab on the target browser.
	conn, _, cleanup, err := browserfixt.SetUpWithURL(ctx, cr, browserType, "about:blank")
	if err != nil {
		s.Fatal("Failed to open a blank new tab: ", err)
	}
	defer cleanup(cleanupCtx)
	defer conn.Close()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get test API connection: ", err)
	}

	w, err := ash.WaitForAnyWindow(ctx, tconn, ash.BrowserTypeMatch(browserType))
	if err != nil {
		s.Fatal("Failed to open a browser window: ", err)
	}
	if err := ash.SetWindowStateAndWait(ctx, tconn, w.ID, ash.WindowStateMaximized); err != nil {
		s.Fatal("Failed to maximize the browser window: ", err)
	}

	rec := power.NewRecorder(ctx, pnp.PNPTimeParams.Interval, s.OutDir(), s.TestName())
	defer rec.Close(cleanupCtx)
	if err := rec.Cooldown(ctx); err != nil {
		s.Fatal("Cooldown failed: ", err)
	}
	if err := rec.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}

	// GoBigSleepLint: Collecting power metrics.
	if err := testing.Sleep(ctx, pnp.PNPTimeParams.Total); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	if err := rec.Finish(ctx); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}
}
