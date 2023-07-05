// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/cswitch"
	"go.chromium.org/tast-tests/cros/common/usbutils"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/typecutils"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ExternalDisplayMirrorModeVideoOrientation,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies video playback in mirror mode on external typec HDMI display with video orientations",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291",
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"intel.cSwitchPort", "intel.domainIP"},
		Data:         []string{"1080p_60fps_600frames.vp8.webm", "video.html", "playback.js"},
		Fixture:      "chromeLoggedIn",
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Timeout:      5 * time.Minute,
	})
}

// ExternalDisplayMirrorModeVideoOrientation requires the following H/W topology to run.
// DUT ---> C-Switch(device that performs hot plug-unplug) ---> Typec HDMI Display.
func ExternalDisplayMirrorModeVideoOrientation(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	cr := s.FixtValue().(*chrome.Chrome)
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	// The Type-C HDMI is connected to C-Switch in P1 as per the intel_cswitch_set1 suite setup.
	cswitchVar := "1"
	if cswitchON, ok := s.Var("intel.cSwitchPort"); ok {
		cswitchVar = cswitchON
	}
	// IP address of Tqc server hosting device.
	domainIP := s.RequiredVar("intel.domainIP")

	// Create C-Switch session that performs hot plug-unplug on TBT/USB4 device.
	sessionID, err := cswitch.CreateSession(ctx, domainIP)
	if err != nil {
		s.Fatal("Failed to create sessionID: ", err)
	}
	defer cswitch.CloseSession(cleanupCtx, sessionID, domainIP)

	if err := cswitch.ToggleCSwitchPort(ctx, sessionID, cswitchVar, domainIP); err != nil {
		s.Fatal("Failed to enable c-switch port: ", err)
	}
	const cSwitchOFF = "0"
	defer cswitch.ToggleCSwitchPort(cleanupCtx, sessionID, cSwitchOFF, domainIP)

	const numOfConnectedDisplays = 1
	typecHDMIRe := regexp.MustCompile(`.*DP branch device present.*yes\n.*Type.*HDMI`)
	typecHDMIInfoPattern := []*regexp.Regexp{typecHDMIRe}
	if err := usbutils.ExternalDisplayDetectionForLocal(ctx, numOfConnectedDisplays, typecHDMIInfoPattern); err != nil {
		s.Fatal("Failed to check for connected external HDMI display: ", err)
	}

	infos, err := display.GetInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get display info: ", err)
	}
	if len(infos) < 2 {
		s.Fatal("Failed to get external display info")
	}

	isMirrorMode := true
	if err := typecutils.SetMirrorDisplay(ctx, tconn, isMirrorMode); err != nil {
		s.Fatal("Failed to set external HDMI display to mirror mode: ", err)
	}
	defer typecutils.SetMirrorDisplay(cleanupCtx, tconn, !isMirrorMode)

	srv := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer srv.Close()
	url := srv.URL + "/video.html"
	conn, err := cr.NewConn(ctx, url)
	if err != nil {
		s.Fatal("Failed to load video.html: ", err)
	}
	defer conn.Close()

	internalDisplayID := infos[0].ID
	videoFile := "1080p_60fps_600frames.vp8.webm"
	videoElement := "document.getElementsByTagName('video')[0]"
	var pausedStatus bool

	defer display.SetDisplayRotationSync(cleanupCtx, tconn, internalDisplayID, display.Rotate0)

	expectedAudioNode := "HDMI"
	cras, err := audio.NewCras(ctx)
	if err != nil {
		s.Fatal("Failed to create Cras object: ", err)
	}

	if err := cras.SetActiveNodeByType(ctx, expectedAudioNode); err != nil {
		s.Fatalf("Failed to select active device %s: %v", expectedAudioNode, err)
	}

	audioDeviceName, audioDeviceType, err := cras.SelectedOutputDevice(ctx)
	if err != nil {
		s.Fatal("Failed to get the selected audio device: ", err)
	}

	if audioDeviceType != expectedAudioNode {
		s.Fatalf("Failed to set audio device: got %q; want %q", audioDeviceType, expectedAudioNode)
	}

	rotationAngles := []display.RotationAngle{display.Rotate90, display.Rotate180, display.Rotate270, display.Rotate0}
	for _, angle := range rotationAngles {
		if err := display.SetDisplayRotationSync(ctx, tconn, internalDisplayID, angle); err != nil {
			s.Fatalf("Failed to rotate to %v angle: %v", angle, err)
		}

		if err := conn.Call(ctx, nil, "playRepeatedly", videoFile, true, true); err != nil {
			s.Fatal("Failed to play video: ", err)
		}

		if err := verifyFirstRunningDevice(ctx, audioDeviceName); err != nil {
			s.Fatal("Failed to verify first running device: ", err)
		}

		// Pause video playback.
		if err := conn.Eval(ctx, videoElement+".pause()", nil); err != nil {
			s.Fatal("Failed to pause video: ", err)
		}
		if err := conn.Eval(ctx, videoElement+".paused", &pausedStatus); err != nil {
			s.Fatal("Failed to pause video: ", err)
		}
		if !pausedStatus {
			s.Fatal("Failed: video is not paused")
		}

		// Again play the paused video.
		if err := conn.Call(ctx, nil, "playRepeatedly", videoFile, true, true); err != nil {
			s.Fatal("Failed to play video: ", err)
		}
		if err := conn.Eval(ctx, videoElement+".paused", &pausedStatus); err != nil {
			s.Fatal("Failed to play video: ", err)
		}
		if pausedStatus {
			s.Fatal("Failed: video is not playing")
		}

		// Checking for frame drops.
		if err := typecutils.CheckFrameDrops(ctx, conn, videoElement, s.OutDir()); err != nil {
			s.Fatal("Failed to check frame drops of playing video: ", err)
		}
		if err := typecutils.VerifyMirrorMode(ctx, tconn); err != nil {
			s.Fatal("Failed to verify mirror mode: ", err)
		}
	}
}

// verifyFirstRunningDevice verifies whether audio is routing through audioDeviceName or not.
func verifyFirstRunningDevice(ctx context.Context, audioDeviceName string) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		devName, err := crastestclient.FirstRunningDevice(ctx, audio.OutputStream)
		if err != nil {
			return errors.Wrap(err, "failed to detect running output device")
		}
		if audioDeviceName != devName {
			return errors.Wrapf(err, "unexpected audio node: got %q; want %q", devName, audioDeviceName)
		}
		return nil
	}, &testing.PollOptions{Timeout: 20 * time.Second})
}
