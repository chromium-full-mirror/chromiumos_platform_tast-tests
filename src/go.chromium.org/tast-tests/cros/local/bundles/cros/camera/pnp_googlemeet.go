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

func init() {
	testing.AddTest(&testing.Test{
		Func:         PNPGooglemeet,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Collect power metrics when in a google meet session",
		BugComponent: "b:167281",
		Contacts:     []string{"chromeos-camera-eng@google.com", "esker@chromium.org"},
		VarDeps:      []string{"ui.bond_credentials"},
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild", "group:camera_dependent"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      1*time.Minute + pnp.PNPTimeParams.Total + power.RecorderTimeout,
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

func PNPGooglemeet(ctx context.Context, s *testing.State) {
	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	bt := s.FixtValue().(powersetup.PowerUIFixtureData).Bt
	cr := s.FixtValue().(powersetup.PowerUIFixtureData).Cr

	r := power.NewRecorder(ctx, pnp.PNPTimeParams.Interval, s.OutDir(), s.TestName())
	defer r.Close(cleanupCtx)
	if err := r.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
	}

	testing.ContextLog(ctx, "Opening Meet")
	conn, br, cleanup, err := browserfixt.SetUpWithURL(ctx, cr, bt, chrome.NewTabURL)
	if err != nil {
		s.Fatal("Failed to launch browser: ", err)
	}
	defer cleanup(cleanupCtx)
	defer conn.Close()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get ash tconn: ", err)
	}

	w, err := ash.WaitForAnyWindow(ctx, tconn, ash.BrowserTypeMatch(bt))
	if err != nil {
		s.Fatal("Failed to open a browser window: ", err)
	}
	if err := ash.SetWindowStateAndWait(ctx, tconn, w.ID, ash.WindowStateMaximized); err != nil {
		s.Fatal("Failed to maximize the browser window: ", err)
	}

	var gm *googlemeet.GoogleMeet
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

	gm, err = googlemeet.JoinMeeting(ctx, cr, br, conn, meetingCode,
		map[string]string{
			// Meet can dynamically switch between different segmentation models.
			// Force the same model with the experiment ?e=ForceSegmentationModelVariant::GpuMid.
			"e": "ForceSegmentationModelVariant::GpuMid",
		}, googlemeet.WithAllPermissions)
	if err != nil {
		s.Fatal("Failed to join meeting: ", err)
	}
	defer gm.Close(cleanupCtx)

	// Configure Meeting.
	if err := uiauto.Combine("Configure Google Meet",
		gm.EnterFullScreen,
		gm.MuteIfMicAvailable,
		gm.ChangeSettings(
			// TODO(esker): There are some settings currently in Google Meet
			// currently not in ChromeOS but in MacOS and gLinux, such as
			// "Video Restore" and "Framing." Make sure these effects are
			// disabled when they are rolled out to ChromeOS.
			gm.SetAdjustVideoLighting(false),
			gm.SetSendResolution(googlemeet.ResolutionHD720P),
			gm.SetReceiveResolution(googlemeet.ResolutionHD720P),
		),
		gm.ApplyVideoEffects(gm.SetEffect(googlemeet.NoEffect)),
	)(ctx); err != nil {
		s.Fatal("Failed to configure Meet: ", err)
	}

	testing.ContextLog(ctx, "Letting things settle for 5 seconds")
	// GoBigSleepLint: Allow power and effects to stabilize before taking metrics.
	if err := testing.Sleep(ctx, 5*time.Second); err != nil {
		s.Fatal("Failed to let things settle: ", err)
	}

	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}

	// GoBigSleepLint: Collecting power metrics.
	if err := testing.Sleep(ctx, pnp.PNPTimeParams.Total); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	if err := r.Finish(ctx); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}
}
