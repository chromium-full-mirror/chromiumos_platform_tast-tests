// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/camera/features"
	"go.chromium.org/tast-tests/cros/local/camera/pnp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vctray"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	initTimePNPCCA = 1 * time.Minute
)

type pnpCCAParams struct {
	mode              cca.Mode
	featureToggleConf features.FeatureToggleConf
	effectsConf       *pnp.EffectsParams
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
			Fixture: "ccaPowerTest",
			Val: pnpCCAParams{
				mode: cca.Photo,
			},
		}, {
			Name:    "photo_mode_fake_hal",
			Fixture: "ccaPowerTestWithFakeHALCamera",
			Val: pnpCCAParams{
				mode: cca.Photo,
			},
		}, {
			Name:    "video_mode",
			Fixture: "ccaPowerTest",
			Val: pnpCCAParams{
				mode: cca.Video,
			},
		}, {
			Name:    "video_mode_fake_hal",
			Fixture: "ccaPowerTestWithFakeHALCamera",
			Val: pnpCCAParams{
				mode: cca.Video,
			},
		}, {
			Name:              "face_gcamae_hdrnet_all_off",
			Fixture:           "ccaPowerTest",
			ExtraHardwareDeps: hwdep.D(hwdep.CameraFeature(features.HDRnet, features.GcamAE)),
			ExtraSoftwareDeps: []string{caps.BuiltinMIPICamera},
			Val: pnpCCAParams{
				mode: cca.Video,
				featureToggleConf: features.FeatureToggleConf{
					features.HDRnet:        false,
					features.GcamAE:        false,
					features.FaceDetection: false,
				},
			},
		}, {
			Name:              "face_on_gcamae_hdrnet_off",
			Fixture:           "ccaPowerTest",
			ExtraHardwareDeps: hwdep.D(hwdep.CameraFeature(features.HDRnet, features.GcamAE)),
			ExtraSoftwareDeps: []string{caps.BuiltinMIPICamera},
			Val: pnpCCAParams{
				mode: cca.Video,
				featureToggleConf: features.FeatureToggleConf{
					features.HDRnet:        false,
					features.GcamAE:        false,
					features.FaceDetection: true,
				},
			},
		}, {
			Name:              "gcamae_on_face_hdrnet_off",
			Fixture:           "ccaPowerTest",
			ExtraHardwareDeps: hwdep.D(hwdep.CameraFeature(features.HDRnet, features.GcamAE)),
			ExtraSoftwareDeps: []string{caps.BuiltinMIPICamera},
			Val: pnpCCAParams{
				mode: cca.Video,
				featureToggleConf: features.FeatureToggleConf{
					features.HDRnet:        false,
					features.GcamAE:        true,
					features.FaceDetection: false,
				},
			},
		}, {
			Name:              "hdrnet_on_face_gcamae_off",
			Fixture:           "ccaPowerTest",
			ExtraHardwareDeps: hwdep.D(hwdep.CameraFeature(features.HDRnet, features.GcamAE)),
			ExtraSoftwareDeps: []string{caps.BuiltinMIPICamera},
			Val: pnpCCAParams{
				mode: cca.Video,
				featureToggleConf: features.FeatureToggleConf{
					features.HDRnet:        true,
					features.GcamAE:        false,
					features.FaceDetection: false,
				},
			},
		}, {
			Name:              "vc_background_blur_on",
			Fixture:           "ccaPowerTest",
			ExtraSoftwareDeps: []string{caps.BuiltinMIPICamera, "camera_feature_effects"},
			Val: pnpCCAParams{
				mode: cca.Video,
				effectsConf: &pnp.EffectsParams{
					BlurLevel:      vctray.BackgroundBlurFull,
					RelightEnabled: false,
				},
			},
		}, {
			Name:              "vc_relight_on",
			Fixture:           "ccaPowerTest",
			ExtraSoftwareDeps: []string{caps.BuiltinMIPICamera, "camera_feature_effects"},
			Val: pnpCCAParams{
				mode: cca.Video,
				effectsConf: &pnp.EffectsParams{
					BlurLevel:      vctray.BackgroundBlurOff,
					RelightEnabled: true,
				},
			},
		}, {
			Name:              "vc_background_blur_relight_on",
			Fixture:           "ccaPowerTest",
			ExtraSoftwareDeps: []string{caps.BuiltinMIPICamera, "camera_feature_effects"},
			Val: pnpCCAParams{
				mode: cca.Video,
				effectsConf: &pnp.EffectsParams{
					BlurLevel:      vctray.BackgroundBlurFull,
					RelightEnabled: true,
				},
			},
		}},
	})
}

func PNPCCAFeatureToggle(ctx context.Context, s *testing.State) {
	// Reserve some time for the cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	pnpRoutine := pnp.Routine{}
	defer pnpRoutine.Close(cleanupCtx)
	if err := pnp.Cooldown(ctx); err != nil {
		s.Fatal("Failed to run pnp cooldown routine: ", err)
	}

	testing.ContextLog(ctx, "[Start Work Phase]")
	if s.Param().(pnpCCAParams).featureToggleConf != nil {
		featureToggler, err := features.NewFeatureToggler(ctx)
		if err != nil {
			s.Fatal("Cannot create feature toggler: ", err)
		}
		defer func() {
			if err := featureToggler.CleanUp(); err != nil {
				s.Error("Cannot close feature toggler: ", err)
			}
		}()
		featureToggler.Toggle(ctx, s.Param().(pnpCCAParams).featureToggleConf)
		if err := upstart.RestartJob(ctx, "cros-camera"); err != nil {
			s.Fatal("Failed to restart cros-camera service: ", err)
		}
	}

	startApp := s.FixtValue().(cca.FixtureData).StartApp
	stopApp := s.FixtValue().(cca.FixtureData).StopApp
	cr := s.FixtValue().(cca.FixtureData).Chrome
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to Chrome: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	app, err := startApp(ctx)
	if err != nil {
		s.Fatal("Failed to open CCA: ", err)
	}
	defer func(cleanupCtx context.Context) {
		if err := stopApp(cleanupCtx, s.HasError()); err != nil {
			s.Fatal("Failed to close CCA: ", err)
		}
	}(cleanupCtx)

	if effectsConf := s.Param().(pnpCCAParams).effectsConf; effectsConf != nil {
		cr := s.FixtValue().(cca.FixtureData).Chrome
		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			s.Fatal("Failed to connect to the test API: ", err)
		}
		vcTray := vctray.New(ctx, tconn)

		// Set camera effects.
		if err := vcTray.SetCameraEffects(effectsConf.BlurLevel, effectsConf.RelightEnabled)(ctx); err != nil {
			s.Fatalf("Failed to set camera effects to BackgroundBlur %v; PortraitRelighting %v: %v",
				effectsConf.BlurLevel, effectsConf.RelightEnabled, err)
		}
	}

	if err := app.FullscreenWindow(ctx); err != nil {
		s.Fatal("Failed to enter full screen of CCA: ", err)
	}
	mode := s.Param().(pnpCCAParams).mode
	if err := app.SwitchMode(ctx, mode); err != nil {
		s.Error("Failed to switch mode ", mode, ": ", err)
	}

	if err := pnp.WarmUp(ctx); err != nil {
		s.Fatal("Failed to run pnp warm up routine: ", err)
	}

	fpsObserver, err := app.FPSObserver(ctx)
	if err != nil {
		s.Fatal("Failed to get FPS observer: ", err)
	}
	defer fpsObserver.Stop(cleanupCtx)

	if err := pnpRoutine.MeasurePower(ctx, cleanupCtx, s.OutDir(), s.TestName(), false); err != nil {
		s.Fatal("Failed to run pnp power measuring routine: ", err)
	}

	fps, err := fpsObserver.AverageFPS(ctx)
	if err != nil {
		s.Fatal("Failed to measure average FPS: ", err)
	}
	pv := perf.NewValues()
	pv.Set(perf.Metric{
		Name:      "cca_fps",
		Unit:      "fps",
		Direction: perf.BiggerIsBetter,
	}, fps)

	if err := pnpRoutine.UploadMetrics(ctx, cleanupCtx, pv); err != nil {
		s.Fatal("Failed to upload metrics: ", err)
	}
}
