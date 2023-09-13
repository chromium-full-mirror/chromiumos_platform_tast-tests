// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/common/usbutils"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/hdcputils"
	"go.chromium.org/tast-tests/cros/local/hdcputils/setup"
	"go.chromium.org/tast-tests/cros/local/hdcputils/urlconst"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/typecutils"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// securityVerifyTestParams struct captures parameters for the SecurityVerifyType1HDCP test.
type securityVerifyTestParams struct {
	displayMode string
	contentURL  string
}

var (
	// hdmiDisplayInfoRe is regexp pattern for verifying HDMI display.
	hdmiDisplayInfoRe = regexp.MustCompile(`\[CONNECTOR:\d+:DP.*status: connected((.|\n)*)DP branch device present: no`)
	// hdcpVerRe is regexp pattern for verifying HDCP 1.4 display.
	hdcpVerRe = regexp.MustCompile(`HDCP1\.4`)
)

func init() {
	testing.AddTest(&testing.Test{
		// Test H/W topology requires DUT connected to external typec DP display.
		Func:         SecurityVerifyType1HDCP,
		Desc:         "Verifies security verification of Type 1 content on HDCP 1.4 compatible panel in extended/mirror mode",
		LacrosStatus: testing.LacrosVariantUnneeded,
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291",
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeLoggedInRootfsRemoved",
		HardwareDeps: hwdep.D(setup.PerfHDCPDevices()),
		Attr:         []string{"group:intel-dp-type-c"},
		Params: []testing.Param{{
			Name: "h264_subsample",
			Val: securityVerifyTestParams{
				displayMode: "mirror",
				contentURL:  urlconst.H264Subsample,
			},
			Timeout: 7 * time.Minute,
		}, {
			Name: "h264_uhd",
			Val: securityVerifyTestParams{
				displayMode: "extended",
				contentURL:  urlconst.H264UHD,
			},
			Timeout: 7 * time.Minute,
		}},
	})
}

func SecurityVerifyType1HDCP(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to the test API connection: ", err)
	}

	testData := s.Param().(securityVerifyTestParams)

	tempDir, err := ioutil.TempDir("", "temp")
	if err != nil {
		s.Fatal("Failed to create temp directory: ", err)
	}
	defer os.RemoveAll(tempDir)
	tmpFilePath := path.Join(tempDir, "dmesgLogs.txt")

	videoConn, err := hdcputils.LaunchShakaPlayer(ctx, cr, testData.contentURL, urlconst.ProxyHDCPV2)
	if err != nil {
		s.Fatal("Failed to launch shaka player: ", err)
	}

	if err := testexec.CommandContext(ctx, "dmesg", "-C").Run(); err != nil {
		s.Error("Failed to clear dmesg log: ", err)
	}

	const numOfConnectedDisplays = 1
	typecDisplayInfoPattern := []*regexp.Regexp{hdmiDisplayInfoRe, hdcpVerRe}
	if err := usbutils.ExternalDisplayDetectionForLocal(ctx, numOfConnectedDisplays, typecDisplayInfoPattern); err != nil {
		s.Fatal("Failed to check for connected external display: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get the keyboard: ", err)
	}

	if testData.displayMode == "extended" {
		isExtDisplay := true
		// Set display in extended mode.
		if err := cuj.SwitchWindowToDisplay(ctx, tconn, kb, isExtDisplay)(ctx); err != nil {
			s.Fatal("Failed to set display in extended mode: ", err)
		}

		defer func(ctx context.Context) {
			notExtDisplay := false
			if err := cuj.SwitchWindowToDisplay(ctx, tconn, kb, notExtDisplay)(ctx); err != nil {
				s.Error("Failed to unset display from extended mode: ", err)
			}
		}(cleanupCtx)

	} else if testData.displayMode == "mirror" {
		setMirrorMode := true
		// Set display in mirror mode.
		if err := typecutils.SetMirrorDisplay(ctx, tconn, setMirrorMode); err != nil {
			s.Fatal("Failed to set mirror mode: ", err)
		}

		defer func(ctx context.Context) {
			unsetMirrorMode := false
			if err := typecutils.SetMirrorDisplay(ctx, tconn, unsetMirrorMode); err != nil {
				s.Error("Failed to unset display from mirror mode: ", err)
			}
		}(cleanupCtx)

		if err := typecutils.VerifyMirrorMode(ctx, tconn); err != nil {
			s.Fatal("Failed to verify mirror mode: ", err)
		}
	}

	setDrmCommand := "echo 0xe > /sys/module/drm/parameters/debug"
	if err := testexec.CommandContext(ctx, "sh", "-c", setDrmCommand).Run(); err != nil {
		s.Fatal("Failed to set drm: ", err)
	}

	defer func(ctx context.Context) {
		unsetDrmCommand := "echo 0 > /sys/module/drm/parameters/debug"
		if err := testexec.CommandContext(ctx, "sh", "-c", unsetDrmCommand).Run(); err != nil {
			s.Error("Failed to unset drm: ", err)
		}
	}(cleanupCtx)

	// Capture drm logs from the command 'dmesg -w' and write into file.
	cmd := testexec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("dmesg -w >> %s &", tmpFilePath))
	if err := cmd.Start(); err != nil {
		s.Fatal("Failed to start dmesg: ", err)
	}
	defer cmd.Kill()

	const videoRobustnessName = "HW_SECURE_ALL"
	if err := videoConn.SelectVideoRobustness(ctx, videoRobustnessName); err != nil {
		s.Fatal("Failed to select HW_SECURE_ALL: ", err)
	}

	// Now click on the green play button.
	// Check if audio is coming while video playback.
	if err := videoConn.PlayVideo(ctx); err != nil {
		s.Fatal("Failed to play video: ", err)
	}

	cras, err := audio.NewCras(ctx)
	if err != nil {
		s.Fatal("Failed to create Cras object: ", err)
	}

	const audioSpeaker = "HDMI"
	videoDuration := 5 // seconds.
	if err := videoConn.VerifyVideoPlayWithDuration(ctx, videoDuration, cras, audioSpeaker); err != nil {
		s.Fatal("Failed to play verify video or audio routing: ", err)
	}
	videoDuration1 := 15 // seconds.
	if err := videoConn.VerifyVideoPlayWithDuration(ctx, videoDuration1, cras, audioSpeaker); err == nil {
		s.Fatal("Failed to pause video: ", err)
	}

	// For display tests:
	// Check if HDCP is enabled message is coming or not,
	// from drm logs when video is played.
	// Execute: dmesg -w | grep hdcp.
	err = testing.Poll(ctx, func(ctx context.Context) error {
		output, err := ioutil.ReadFile(tmpFilePath)
		if err != nil {
			return testing.PollBreak(errors.Wrapf(err, "failed to read %s drm log file", tmpFilePath))
		}
		re := regexp.MustCompile(`HDCP\d.\d is enabled. Type \d`)
		if re.MatchString(string(output)) {
			return errors.New("Drm logs found in dmesg")
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: 1 * time.Second})

	if err != nil {
		s.Fatal("Failed to verify HDCP is enabled message from drm logs: ", err)
	}
}
