// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/common/usbutils"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
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

type playH264AV1Params struct {
	contentURL string
	hdcpVer    string
	drmLogMsg  string
}

func init() {
	// The test requires that the DUT connects to an external Type-C DP 4K display.
	testing.AddTest(&testing.Test{
		Func:         PlayH264AV1OnExternalDisplay,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies HDCP session with playing video and changing codec in extended mode",
		Contacts:     []string{"intel-chrome-system-automation-team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291",
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"intel.videoDuration"},
		Fixture:      "chromeLoggedInRootfsRemoved",
		HardwareDeps: hwdep.D(setup.PerfHDCPDevices()),
		Attr:         []string{"group:intel-dp-type-c"},
		Params: []testing.Param{{
			Name: "h264_subsample",
			Val: playH264AV1Params{
				contentURL: urlconst.H264Fullsample,
				hdcpVer:    "HDCP2.2",
				drmLogMsg:  "HDCP2.2 is enabled. Type 0",
			},
			Timeout: 7 * time.Minute,
		}, {
			Name: "av1",
			Val: playH264AV1Params{
				contentURL: urlconst.H264Fullsample,
				hdcpVer:    "HDCP1.4",
				drmLogMsg:  "HDCP is enabled",
			},
			Timeout: 7 * time.Minute,
		}},
	})
}

func PlayH264AV1OnExternalDisplay(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	cr := s.FixtValue().(*chrome.Chrome)
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}

	testData := s.Param().(playH264AV1Params)

	cras, err := audio.NewCras(ctx)
	if err != nil {
		s.Fatal("Failed to create Cras object: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create keyboard eventwriter: ", err)
	}

	// Mute the device to avoid noisiness.
	if err := crastestclient.Mute(ctx); err != nil {
		s.Fatal("Failed to mute: ", err)
	}
	defer crastestclient.Unmute(cleanupCtx)

	const isExtDisplay = true
	defer cuj.SwitchWindowToDisplay(cleanupCtx, tconn, kb, !isExtDisplay)(ctx)

	videoConn, err := hdcputils.LaunchShakaPlayer(ctx, cr, testData.contentURL, urlconst.ProxyHDCPV1)
	if err != nil {
		s.Fatal("Failed to launch shaka player: ", err)
	}

	displayInfoRe := regexp.MustCompile(`\[CONNECTOR:\d+:DP.*status: connected((.|\n)*)DP branch device present: no`)
	hdcpVerRe := regexp.MustCompile(testData.hdcpVer)

	typecDisplayInfoPattern := []*regexp.Regexp{displayInfoRe, hdcpVerRe}
	const numOfConnectedDisplays = 1
	if err := usbutils.ExternalDisplayDetectionForLocal(ctx, numOfConnectedDisplays, typecDisplayInfoPattern); err != nil {
		s.Fatal("Failed to check for a connected external display: ", err)
	}

	if testData.hdcpVer == "HDCP2.2" {
		if err := typecutils.VerifyDisplay4KResolution(ctx); err != nil {
			s.Fatal("Failed to verify display at 4K resolution: ", err)
		}
	}

	if err := cuj.SwitchWindowToDisplay(ctx, tconn, kb, isExtDisplay)(ctx); err != nil {
		s.Fatal("Failed to set the display in extended mode: ", err)
	}

	const DrmPath = "/sys/module/drm/parameters/debug"
	func() {
		f, err := os.OpenFile(DrmPath, os.O_WRONLY, 0777)
		if err != nil {
			s.Fatal("Failed to set DRM: ", err)
		}
		defer func() {
			if err := f.Close(); err != nil {
				s.Fatal("Failed to close file: ", err)
			}
		}()
		set := []byte("0xe")
		if _, err := f.Write(set); err != nil {
			s.Fatal("Failed to write on to file: ", err)
		}
	}()

	defer func() {
		f, err := os.OpenFile(DrmPath, os.O_WRONLY, 0777)
		if err != nil {
			s.Fatal("Failed to set DRM: ", err)
		}
		unset := []byte("0")
		if _, err := f.Write(unset); err != nil {
			s.Fatal("Failed to write on to file: ", err)
		}
		if err := f.Close(); err != nil {
			s.Fatal("Failed to close file: ", err)
		}
	}()

	// Select HW_SECURE_ALL in video robustness from the gear icon.
	const videoRobustnessName = "HW_SECURE_ALL"
	if err := videoConn.SelectVideoRobustness(ctx, videoRobustnessName); err != nil {
		s.Fatal("Failed to select HW_SECURE_ALL: ", err)
	}

	// Now click on the green play button.
	// Check if audio is coming while video playback.
	if err := videoConn.PlayVideo(ctx); err != nil {
		s.Fatal("Failed to play video: ", err)
	}

	videoDuration := 16
	if duration, ok := s.Var("intel.videoDuration"); ok {
		videoDuration, err = strconv.Atoi(duration)
		if err != nil {
			s.Fatal("Failed to convert string to integer: ", err)
		}
	}

	// For display tests:
	// Check if HDCP enabled message is coming or not,
	// from drm logs when video is played.
	// Execute: dmesg | grep hdcp.

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Clear dmesg before taking any logs.
		if err := testexec.CommandContext(ctx, "dmesg", "-C").Run(); err != nil {
			s.Fatal("Failed to clear dmesg log: ", err)
		}
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			cmd := testexec.CommandContext(ctx, "dmesg")
			output, err := cmd.Output(testexec.DumpLogOnError)
			if err != nil {
				return errors.Wrap(err, "failed to execute grep hdcp command")
			}
			if !strings.Contains(string(output), testData.drmLogMsg) {
				return errors.New("failed to verify drm logs")
			}
			return nil
		}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: 1 * time.Second}); err != nil {
			s.Fatal("Failed to verify HDCP is enabled message from drm logs: ", err)
		}
	}()

	wg.Wait()
	const expectedAudioNode = "HDMI"
	if err := videoConn.VerifyVideoPlayWithDuration(ctx, videoDuration, cras, expectedAudioNode); err != nil {
		s.Fatal("Failed to play verify video or audio routing: ", err)
	}
}
