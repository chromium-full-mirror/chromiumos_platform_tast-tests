// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"time"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast-tests/cros/common/bond"
	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/camera/pnp"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/thirdparty/googlemeet"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/power"
	powersetup "go.chromium.org/tast-tests/cros/local/power/setup"
)

const (
	initTimePNPGoogleMeet = 1 * time.Minute
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PNPGoogleMeet,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Collect power metrics when in a google meet session",
		Contacts:     []string{"chromeos-camera-eng@google.com", "esker@chromium.org"},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		VarDeps:      []string{"ui.bond_credentials"},
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild", "group:camera_dependent"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      initTimePNPGoogleMeet + pnp.PNPTimeParams.Total + power.RecorderTimeout,
		Params: []testing.Param{{
			Name:              "ash",
			Fixture:           pnp.StablePowerAshGAIA,
			ExtraSoftwareDeps: []string{caps.BuiltinCamera},
		}, {
			Name:    "ash_fake_hal",
			Fixture: pnp.StablePowerAshGAIAFakeHAL,
		}, {
			Name:              "lacros",
			Fixture:           pnp.StablePowerLacrosGAIA,
			ExtraSoftwareDeps: []string{caps.BuiltinCamera},
		}, {
			Name:    "lacros_fake_hal",
			Fixture: pnp.StablePowerLacrosGAIAFakeHAL,
		}},
	})
}

func PNPGoogleMeet(ctx context.Context, s *testing.State) {
	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

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
