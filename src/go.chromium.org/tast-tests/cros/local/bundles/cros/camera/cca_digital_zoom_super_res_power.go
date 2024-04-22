// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/power"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type digitalZoomPowerSubtest struct {
	useZoom bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCADigitalZoomSuperResPower,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Collect power metrics with digital zoom and super resolution flag",
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Contacts:     []string{"chromeos-camera-eng@google.com", "kamchonlathorn@chromium.org"},
		SoftwareDeps: []string{"chrome", "camera_app", caps.BuiltinCamera},
		Timeout:      6*time.Minute + power.RecorderTimeout,
		Params: []testing.Param{{
			Name:      "disabled",
			Fixture:   "ccaPowerReview",
			ExtraAttr: []string{"group:crosbolt", "crosbolt_nightly"},
			Val:       digitalZoomPowerSubtest{useZoom: false},
		}, {
			Name:      "enabled_zoom_inactive",
			Fixture:   "ccaPowerReviewWithDigitalZoomSuperResEnabled",
			ExtraAttr: []string{"group:crosbolt", "crosbolt_nightly"},
			Val:       digitalZoomPowerSubtest{useZoom: false},
		}, {
			Name:      "enabled_zoom_active",
			Fixture:   "ccaPowerReviewWithDigitalZoomSuperResEnabled",
			ExtraAttr: []string{"group:crosbolt", "crosbolt_nightly"},
			Val:       digitalZoomPowerSubtest{useZoom: true},
		}},
	})
}

func CCADigitalZoomSuperResPower(ctx context.Context, s *testing.State) {
	startApp := s.FixtValue().(cca.FixtureData).StartApp
	stopApp := s.FixtValue().(cca.FixtureData).StopApp

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	interval := cca.PowerTimeParams.Interval
	total := cca.PowerTimeParams.Total

	r := power.NewRecorder(ctx, interval, s.OutDir(), s.TestName())
	defer r.Close(cleanupCtx)

	if err := power.Cooldown(ctx); err != nil {
		s.Log("Failed to cooldown before the setup: ", err)
	}

	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}

	app, err := startApp(ctx)
	if err != nil {
		s.Fatal("Failed to open CCA: ", err)
	}
	defer func(cleanupCtx context.Context) {
		if err := stopApp(cleanupCtx, s.HasError()); err != nil {
			s.Fatal("Failed to close CCA: ", err)
		}
	}(cleanupCtx)

	if s.Param().(digitalZoomPowerSubtest).useZoom {
		if err := app.ZoomInFromPTZPanel(ctx); err != nil {
			s.Fatal("Failed to zoom in from PTZ panel: ", err)
		}
	}

	// GoBigSleepLint: Keep the camera stream to measure the power usage.
	if err := testing.Sleep(ctx, total); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}
	if err := r.Finish(ctx); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}
}
