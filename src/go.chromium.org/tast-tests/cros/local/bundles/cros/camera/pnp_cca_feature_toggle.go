// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/camera/pnp"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

const (
	initTimePNPCCA = 1 * time.Minute
)

type pnpCCAParams struct {
	Mode cca.Mode
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         PNPCCAFeatureToggle,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Collect power metrics for CCA",
		Contacts:     []string{"chromeos-camera-eng@google.com", "esker@chromium.org"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild", "group:camera_dependent"},
		SoftwareDeps: []string{caps.BuiltinCamera, "chrome", "camera_app"},
		Timeout:      initTimePNPCCA + pnp.PNPTimeParams.Total + power.RecorderTimeout,
		Params: []testing.Param{{
			Name:    "photo_mode",
			Fixture: "ccaLaunchedStableEnv",
			Val: pnpCCAParams{
				Mode: cca.Photo,
			},
		}, {
			Name:    "photo_mode_fake_hal",
			Fixture: "ccaLaunchedStableEnvFakeHALCamera",
			Val: pnpCCAParams{
				Mode: cca.Photo,
			},
		}, {
			Name:    "video_mode",
			Fixture: "ccaLaunchedStableEnv",
			Val: pnpCCAParams{
				Mode: cca.Video,
			},
		}, {
			Name:    "video_mode_fake_hal",
			Fixture: "ccaLaunchedStableEnvFakeHALCamera",
			Val: pnpCCAParams{
				Mode: cca.Video,
			},
		}},
	})
}

func PNPCCAFeatureToggle(ctx context.Context, s *testing.State) {
	// Reserve some time for the cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	if err := pnp.Cooldown(ctx); err != nil {
		s.Fatal("Failed to run pnp cooldown routine: ", err)
	}

	testing.ContextLog(ctx, "[Start Work Phase]")
	app := s.FixtValue().(cca.FixtureData).App()

	if err := app.FullscreenWindow(ctx); err != nil {
		s.Fatal("Failed to enter full screen of CCA: ", err)
	}

	mode := s.Param().(pnpCCAParams).Mode
	if err := app.SwitchMode(ctx, mode); err != nil {
		s.Error("Failed to switch mode ", mode, ": ", err)
	}

	if err := pnp.WarmUp(ctx); err != nil {
		s.Fatal("Failed to run pnp warm up routine: ", err)
	}
	if err := pnp.MeasurePower(ctx, cleanupCtx, s.OutDir(), s.TestName()); err != nil {
		s.Fatal("Failed to run pnp power measuring routine: ", err)
	}

}
