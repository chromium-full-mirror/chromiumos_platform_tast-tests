// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"path/filepath"
	"time"

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
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

const (
	traceTimePNPGoogleMeetFrameCapture = 20 * time.Second
	initTimePNPGoogleMeetFrameCapture  = 1 * time.Minute
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PNPGoogleMeetFrameCapture,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Collect latency information of frame capturing functions during a Google Meet session",
		Contacts:     []string{"chromeos-camera-eng@google.com", "esker@chromium.org"},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		VarDeps:      []string{"ui.bond_credentials"},
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{caps.BuiltinCamera, "chrome"},
		Fixture:      pnp.StablePowerLacrosGAIA,
		Timeout:      initTimePNPGoogleMeetFrameCapture + traceTimePNPGoogleMeetFrameCapture + power.RecorderTimeout,
	})
}

func PNPGoogleMeetFrameCapture(ctx context.Context, s *testing.State) {
	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

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

	// Start Perfetto.
	frameCaptureRecorder := pnp.FrameCaptureRecorder{}
	if err := frameCaptureRecorder.StartRecording(ctx, filepath.Join(s.OutDir(), "trace.pb")); err != nil {
		s.Fatal("Failed start recording: ", err)
	}
	defer func() {
		// GoBigSleepLint: Cros-camera device closing function adheres to the Android
		// Camera3 API, which mandates a 500 ms response time.
		if err := testing.Sleep(cleanupCtx, 500*time.Millisecond); err != nil {
			s.Fatal("Failed to sleep: ", err)
		}

		if err := frameCaptureRecorder.StopRecording(cleanupCtx); err != nil {
			s.Fatal("Failed stop recording: ", err)
		}

		if s.HasError() {
			return
		}
		if err := frameCaptureRecorder.CollectMetrics(cleanupCtx, s.OutDir()); err != nil {
			s.Fatal("Failed collect metrics: ", err)
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

	testing.ContextLog(ctx, "[Record Phase] Start recording trace for ", traceTimePNPGoogleMeetFrameCapture)
	// GoBigSleepLint: Collect performance metrics.
	if err := testing.Sleep(ctx, traceTimePNPGoogleMeetFrameCapture); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}
}
