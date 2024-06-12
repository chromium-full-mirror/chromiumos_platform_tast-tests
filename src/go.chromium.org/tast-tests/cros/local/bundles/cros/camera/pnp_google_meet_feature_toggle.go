// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/bond"
	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/camera/features"
	"go.chromium.org/tast-tests/cros/local/camera/pnp"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/thirdparty/googlemeet"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vctray"
	"go.chromium.org/tast-tests/cros/local/power"
	powersetup "go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	initTimePNPGoogleMeetFeatureToggle = 1 * time.Minute
)

type pnpGoogleMeetFeatureToggleParams struct {
	featureToggleConf features.FeatureToggleConf
	effectsConf       *pnp.EffectsParams
	cameraOff         bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         PNPGoogleMeetFeatureToggle,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Collect power metrics when in a google meet session",
		Contacts:     []string{"chromeos-camera-eng@google.com", "esker@chromium.org"},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		VarDeps:      []string{"ui.bond_credentials"},
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      initTimePNPGoogleMeetFeatureToggle + pnp.PNPTimeParams.Total + power.RecorderTimeout,
		Params: []testing.Param{{
			Name:              "ash",
			Fixture:           pnp.StablePowerAshGAIA,
			ExtraSoftwareDeps: []string{caps.BuiltinCamera},
			Val:               pnpGoogleMeetFeatureToggleParams{},
		}, {
			Name:    "ash_fake_hal",
			Fixture: pnp.StablePowerAshGAIAFakeHAL,
			Val:     pnpGoogleMeetFeatureToggleParams{},
		}, {
			Name:    "ash_camera_off",
			Fixture: pnp.StablePowerAshGAIA,
			Val:     pnpGoogleMeetFeatureToggleParams{cameraOff: true},
		}, {
			Name:              "lacros",
			Fixture:           pnp.StablePowerLacrosGAIA,
			ExtraSoftwareDeps: []string{caps.BuiltinCamera},
			Val:               pnpGoogleMeetFeatureToggleParams{},
		}, {
			Name:    "lacros_fake_hal",
			Fixture: pnp.StablePowerLacrosGAIAFakeHAL,
			Val:     pnpGoogleMeetFeatureToggleParams{},
		}, {
			Name:    "lacros_camera_off",
			Fixture: pnp.StablePowerLacrosGAIA,
			Val:     pnpGoogleMeetFeatureToggleParams{cameraOff: true},
		}, {
			Name:              "lacros_face_gcamae_hdrnet_all_off",
			Fixture:           pnp.StablePowerLacrosGAIA,
			ExtraHardwareDeps: hwdep.D(hwdep.CameraFeature(features.HDRnet, features.GcamAE)),
			ExtraSoftwareDeps: []string{caps.BuiltinMIPICamera},
			Val: pnpGoogleMeetFeatureToggleParams{
				featureToggleConf: features.FeatureToggleConf{
					features.HDRnet:        false,
					features.GcamAE:        false,
					features.FaceDetection: false,
				},
			},
		}, {
			Name:              "lacros_face_on_gcamae_hdrnet_off",
			Fixture:           pnp.StablePowerLacrosGAIA,
			ExtraHardwareDeps: hwdep.D(hwdep.CameraFeature(features.HDRnet, features.GcamAE)),
			ExtraSoftwareDeps: []string{caps.BuiltinMIPICamera},
			Val: pnpGoogleMeetFeatureToggleParams{
				featureToggleConf: features.FeatureToggleConf{
					features.HDRnet:        false,
					features.GcamAE:        false,
					features.FaceDetection: true,
				},
			},
		}, {
			Name:              "lacros_gcamae_on_face_hdrnet_off",
			Fixture:           pnp.StablePowerLacrosGAIA,
			ExtraHardwareDeps: hwdep.D(hwdep.CameraFeature(features.HDRnet, features.GcamAE)),
			ExtraSoftwareDeps: []string{caps.BuiltinMIPICamera},
			Val: pnpGoogleMeetFeatureToggleParams{
				featureToggleConf: features.FeatureToggleConf{
					features.HDRnet:        false,
					features.GcamAE:        true,
					features.FaceDetection: false,
				},
			},
		}, {
			Name:              "lacros_hdrnet_on_face_gcamae_off",
			Fixture:           pnp.StablePowerLacrosGAIA,
			ExtraHardwareDeps: hwdep.D(hwdep.CameraFeature(features.HDRnet, features.GcamAE)),
			ExtraSoftwareDeps: []string{caps.BuiltinMIPICamera},
			Val: pnpGoogleMeetFeatureToggleParams{
				featureToggleConf: features.FeatureToggleConf{
					features.HDRnet:        true,
					features.GcamAE:        false,
					features.FaceDetection: false,
				},
			},
		}, {
			Name:              "lacros_vc_backgroun_blur_on",
			Fixture:           pnp.StablePowerLacrosGAIA,
			ExtraSoftwareDeps: []string{"camera_feature_effects"},
			ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)), // Only enable on CBX devices.
			Val: pnpGoogleMeetFeatureToggleParams{
				effectsConf: &pnp.EffectsParams{
					BlurLevel:      vctray.BackgroundBlurFull,
					RelightEnabled: false,
				},
			},
		}, {
			Name:              "lacros_vc_relight_on",
			Fixture:           pnp.StablePowerLacrosGAIA,
			ExtraSoftwareDeps: []string{"camera_feature_effects"},
			ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)), // Only enable on CBX devices.
			Val: pnpGoogleMeetFeatureToggleParams{
				effectsConf: &pnp.EffectsParams{
					BlurLevel:      vctray.BackgroundBlurOff,
					RelightEnabled: true,
				},
			},
		}, {
			Name:              "lacros_vc_background_blur_relight_on",
			Fixture:           pnp.StablePowerLacrosGAIA,
			ExtraSoftwareDeps: []string{"camera_feature_effects"},
			ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)), // Only enable on CBX devices.
			Val: pnpGoogleMeetFeatureToggleParams{
				effectsConf: &pnp.EffectsParams{
					BlurLevel:      vctray.BackgroundBlurFull,
					RelightEnabled: true,
				},
			},
		}},
	})
}

func PNPGoogleMeetFeatureToggle(ctx context.Context, s *testing.State) {
	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	if s.Param().(pnpGoogleMeetFeatureToggleParams).cameraOff {
		if err := upstart.StopJob(ctx, "cros-camera"); err != nil {
			s.Fatal("Failed to stop cros-camera service: ", err)
		}
		defer func() {
			if err := upstart.EnsureJobRunning(ctx, "cros-camera"); err != nil {
				s.Fatal("Failed to start cros-camera service: ", err)
			}
		}()
	}

	pnpRoutine := pnp.Routine{}
	defer pnpRoutine.Close(cleanupCtx)
	if err := pnp.Cooldown(ctx); err != nil {
		s.Fatal("Failed to run pnp cooldown routine: ", err)
	}

	testing.ContextLog(ctx, "[Start Work Phase]")
	browserType := s.FixtValue().(powersetup.PowerUIFixtureData).Bt
	cr := s.FixtValue().(powersetup.PowerUIFixtureData).Cr

	testing.ContextLog(ctx, "Opening Meet")
	conn, br, cleanup, err := browserfixt.SetUpWithURL(ctx, cr, browserType, chrome.NewTabURL)
	if err != nil {
		s.Fatal("Failed to launch browser: ", err)
	}
	defer cleanup(cleanupCtx)
	defer conn.Close()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get ash tconn: ", err)
	}

	if s.Param().(pnpGoogleMeetFeatureToggleParams).featureToggleConf != nil {
		featureToggler, err := features.NewFeatureToggler(ctx)
		if err != nil {
			s.Fatal("Cannot create feature toggler: ", err)
		}
		defer func() {
			if err := featureToggler.CleanUp(); err != nil {
				s.Error("Cannot close feature toggler: ", err)
			}
		}()
		featureToggler.Toggle(ctx, s.Param().(pnpGoogleMeetFeatureToggleParams).featureToggleConf)
		if err := upstart.RestartJob(ctx, "cros-camera"); err != nil {
			s.Fatal("Failed to restart cros-camera service: ", err)
		}
	}

	w, err := ash.WaitForAnyWindow(ctx, tconn, ash.BrowserTypeMatch(browserType))
	if err != nil {
		s.Fatal("Failed to open a browser window: ", err)
	}
	if err := ash.SetWindowStateAndWait(ctx, tconn, w.ID, ash.WindowStateMaximized); err != nil {
		s.Fatal("Failed to maximize the browser window: ", err)
	}

	creds := s.RequiredVar("ui.bond_credentials")
	bc, err := bond.NewClient(ctx, bond.WithCredsJSON([]byte(creds)))
	if err != nil {
		s.Fatal("Failed to create a bond client: ", err)
	}
	defer bc.Close()

	var meetingCode string
	func() {
		sctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		meetingCode, err = bc.CreateConference(sctx)
		if err != nil {
			s.Fatal("Failed to create a conference room: ", err)
		}
	}()

	testing.ContextLog(ctx, "Meeting created with code: ", meetingCode)
	func() {
		sctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		_, _, err := bc.AddBots(sctx, meetingCode, 1, 2*time.Minute+pnp.PNPTimeParams.Total, bond.WithoutVideo())
		if err != nil {
			s.Fatal("Failed to add bots: ", err)
		}
	}()

	var gm *googlemeet.GoogleMeet
	gm, err = googlemeet.JoinMeeting(ctx, cr, br, conn, meetingCode,
		map[string]string{
			// Meet can dynamically switch between different segmentation models.
			// Force the same model with the experiment ?e=ForceSegmentationModelVariant::GpuMid.
			"e": "ForceSegmentationModelVariant::GpuMid",
		}, googlemeet.WithAllPermissions)
	if err != nil {
		s.Fatal("Failed to join a meeting: ", err)
	}
	defer gm.Close(cleanupCtx)

	if effectsConf := s.Param().(pnpGoogleMeetFeatureToggleParams).effectsConf; effectsConf != nil {
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

	// Configure Meeting.
	if err := uiauto.Combine("Configure Google Meet",
		gm.EnterFullScreen,
		gm.MuteIfMicAvailable,
		gm.ChangeSettings(
			gm.SetSendResolution(googlemeet.ResolutionHD720P),
			gm.SetReceiveResolution(googlemeet.ResolutionHD720P),
		),
		// TODO(esker): Make sure to disable all video effect. And there are some
		// settings currently in Google Meet currently not in ChromeOS but in MacOS
		// and gLinux, such as "Video Restore" and "Framing." Make sure these
		// effects are disabled when they are rolled out to ChromeOS.
	)(ctx); err != nil {
		s.Fatal("Failed to configure Meet: ", err)
	}

	if err := pnp.WarmUp(ctx); err != nil {
		s.Fatal("Failed to run pnp warm up routine: ", err)
	}
	if err := pnpRoutine.MeasurePower(ctx, cleanupCtx, s.OutDir(), s.TestName(), true); err != nil {
		s.Fatal("Failed to run pnp power measuring routine: ", err)
	}
}
