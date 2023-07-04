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
	"strings"
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

// videoContent struct stores test specific data.
type videoContent struct {
	contentUrls         []string
	proxyURL            string
	hdcpVer             string
	displayInfoReString string
	typeVal             string
	display             string
	drmLogMsg           string
	is4KDisplay         bool
	isTBTDevice         bool
}

func init() {
	// TODO(b/238157101): We are not running this test on any bots intentionally.
	testing.AddTest(&testing.Test{
		Func:         VerifyVideoContents,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies different widevine secure content using shaka player",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291",
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeLoggedInRootfsRemoved",
		HardwareDeps: hwdep.D(setup.PerfHDCPDevices()),
		Params: []testing.Param{{
			Name: "h264",
			Val: videoContent{contentUrls: []string{urlconst.H264SD, urlconst.H264HD, urlconst.H264UHD, urlconst.H264Fullsample, urlconst.H264CBCS},
				proxyURL: urlconst.ProxyURL},
			Timeout:   9 * time.Minute,
			ExtraAttr: []string{"group:intel-nda"},
		}, {
			Name: "vp9",
			Val: videoContent{contentUrls: []string{urlconst.VP9Subsample, urlconst.VP9Superframe},
				proxyURL: urlconst.ProxyURL},
			Timeout:   4 * time.Minute,
			ExtraAttr: []string{"group:intel-nda"},
		}, {
			Name: "hevc",
			Val: videoContent{contentUrls: []string{urlconst.HEVCclip,
				urlconst.HEVCCBCS, urlconst.HEVC4K, urlconst.HEVCclipSD, urlconst.HEVCclipHD},
				proxyURL: urlconst.ProxyURL},
			Timeout:   9 * time.Minute,
			ExtraAttr: []string{"group:intel-nda"},
		}, {
			// Test H/W topology requires DUT connected to external typec DP 4K display.
			Name: "h264_uhd_hdcpv2_4k_display",
			Val: videoContent{display: "DP",
				drmLogMsg:           "HDCP2.2 is enabled. Type 1",
				hdcpVer:             "HDCP2.2",
				typeVal:             "Type1=1",
				displayInfoReString: `\[CONNECTOR:\d+:DP.*status: connected((.|\n)*)DP branch device present: no`,
				is4KDisplay:         true,
				contentUrls:         []string{urlconst.H264UHD},
				proxyURL:            urlconst.ProxyHDCPV2},
			Timeout:   4 * time.Minute,
			ExtraAttr: []string{"group:intel-dp-type-c"},
		}, {
			// Test H/W topology requires DUT connected to external typec HDMI display.
			Name: "hevc_cbcs_hdcp2_2_v1_display",
			Val: videoContent{display: "HDMI",
				drmLogMsg:           "HDCP2.2 is enabled. Type 0",
				hdcpVer:             "HDCP2.2",
				typeVal:             "Type1=1",
				displayInfoReString: `.*DP branch device present.*yes\n.*Type.*HDMI`,
				contentUrls:         []string{urlconst.HEVCCBCS},
				proxyURL:            urlconst.ProxyHDCPV1},
			Timeout:   4 * time.Minute,
			ExtraAttr: []string{"group:intel-hdmi-type-c"},
		}, {
			// Test H/W topology requires DUT connected to external typec HDMI 4K display.
			Name: "hevc_cbcs_hdcp1_4_v1_display",
			Val: videoContent{display: "HDMI",
				drmLogMsg:           "HDCP2.2 is enabled. Type 0",
				hdcpVer:             "HDCP1.4",
				typeVal:             "Type1=1",
				displayInfoReString: `.*DP branch device present.*yes\n.*Type.*HDMI`,
				is4KDisplay:         true,
				contentUrls:         []string{urlconst.HEVCCBCS},
				proxyURL:            urlconst.ProxyHDCPV1},
			Timeout:   4 * time.Minute,
			ExtraAttr: []string{"group:intel-hdmi-type-c"},
		}, {
			// Test H/W topology requires DUT connected to external typec DP 4K display.
			Name: "h264_hdcpv2_4k_display",
			Val: videoContent{display: "DP",
				drmLogMsg:           "HDCP2.2 is enabled. Type 1",
				hdcpVer:             "HDCP2.2",
				typeVal:             "Type1=1",
				displayInfoReString: `\[CONNECTOR:\d+:DP.*status: connected((.|\n)*)DP branch device present: no`,
				is4KDisplay:         true,
				contentUrls:         []string{urlconst.H264Fullsample, urlconst.H264Subsample, urlconst.H264CBCS},
				proxyURL:            urlconst.ProxyHDCPV2},
			Timeout:   7 * time.Minute,
			ExtraAttr: []string{"group:intel-dp-type-c"},
		}, {
			// Test H/W topology requires DUT connected to external typec HDMI 4K display.
			Name: "vp9_hdcpv2_4k_display",
			Val: videoContent{display: "HDMI",
				drmLogMsg:           "HDCP2.2 is enabled. Type 1",
				hdcpVer:             "HDCP2.2",
				typeVal:             "Type1=1",
				displayInfoReString: `.*DP branch device present.*yes\n.*Type.*HDMI`,
				is4KDisplay:         true,
				contentUrls:         []string{urlconst.VP9Subsample, urlconst.VP9Superframe, urlconst.VP9UHD},
				proxyURL:            urlconst.ProxyHDCPV2},
			Timeout:   7 * time.Minute,
			ExtraAttr: []string{"group:intel-hdmi-type-c"},
		}, {
			// Test H/W topology requires DUT connected to external typec DP 4K display.
			Name: "play_all_hevc_contents_4k_display",
			Val: videoContent{display: "DP",
				drmLogMsg:           "HDCP2.2 is enabled. Type 1",
				hdcpVer:             "HDCP2.2",
				typeVal:             "Type1=1",
				displayInfoReString: `\[CONNECTOR:\d+:DP.*status: connected((.|\n)*)DP branch device present: no`,
				is4KDisplay:         true,
				contentUrls: []string{urlconst.HEVCclip, urlconst.HEVC4K, urlconst.HEVCclipSD, urlconst.HEVCclipHD,
					urlconst.HEVCCBCS, urlconst.HEVCCBCS2},
				proxyURL: urlconst.ProxyHDCPV2},
			Timeout:   12 * time.Minute,
			ExtraAttr: []string{"group:intel-dp-type-c"},
		}, {
			// Test H/W topology required as below:
			// DUT ---> TBT Dock station ---> typec 4K DP display.
			Name: "hevc_hdcp2_tbt_dock_4k_display",
			Val: videoContent{display: "DP",
				drmLogMsg:           "HDCP2.2 is enabled. Type 1",
				hdcpVer:             "HDCP2.2",
				typeVal:             "Type1=1",
				displayInfoReString: `\[CONNECTOR:\d+:DP.*status: connected((.|\n)*)DP branch device present: no`,
				is4KDisplay:         true,
				contentUrls:         []string{urlconst.HEVCCBCS, urlconst.HEVCclip},
				proxyURL:            urlconst.ProxyHDCPV2},
			Timeout:   7 * time.Minute,
			ExtraAttr: []string{"group:intel-dp-type-c"},
		}, {
			// Test H/W topology required as below:
			// DUT ---> USB4 Gatkex ---> typec 4K DP display.
			Name: "vp9_hdcp2_usb4_gatkex_4k_display",
			Val: videoContent{display: "DP",
				drmLogMsg:           "HDCP2.2 is enabled. Type 1",
				hdcpVer:             "HDCP2.2",
				typeVal:             "Type1=1",
				displayInfoReString: `\[CONNECTOR:\d+:DP.*status: connected((.|\n)*)DP branch device present: no`,
				is4KDisplay:         true,
				isTBTDevice:         true,
				contentUrls:         []string{urlconst.VP9Subsample, urlconst.VP9Superframe, urlconst.VP9UHD},
				proxyURL:            urlconst.ProxyHDCPV2},
			Timeout:   7 * time.Minute,
			ExtraAttr: []string{"group:intel-dp-type-c"},
		}},
	})
}

func VerifyVideoContents(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	cr := s.FixtValue().(*chrome.Chrome)
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}

	testData := s.Param().(videoContent)

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

	tempDir, err := ioutil.TempDir("", "temp")
	if err != nil {
		s.Fatal("Failed to create temp directory: ", err)
	}
	defer os.RemoveAll(tempDir)
	tmpFilePath := path.Join(tempDir, "dmesgLogs.txt")

	var (
		expectedAudioNode = "INTERNAL_SPEAKER"
		isExtDisplay      = false
	)

	defer cuj.SwitchWindowToDisplay(cleanupCtx, tconn, kb, isExtDisplay)(ctx)
	for _, contentURL := range testData.contentUrls {
		videoConn, err := hdcputils.LaunchShakaPlayer(ctx, cr, contentURL, testData.proxyURL)
		if err != nil {
			s.Fatal("Failed to launch shaka player: ", err)
		}

		if testData.display != "" {
			// If test requires Thunderbolt dock/USB4 Gatkex check for its detection.
			if testData.isTBTDevice {
				const expected = true
				if err := typecutils.CheckTBTDevice(expected); err != nil {
					s.Fatal("Failed to verify connected Thunderbolt device: ", err)
				}
			}
			// Clear dmesg before taking any logs.
			if err := testexec.CommandContext(ctx, "dmesg", "-C").Run(); err != nil {
				s.Error("Failed to clear dmesg log: ", err)
			}

			isExtDisplay = true
			expectedAudioNode = "HDMI"
			const numOfConnectedDisplays = 1
			displayInfoRe := regexp.MustCompile(testData.displayInfoReString)
			hdcpVerRe := regexp.MustCompile(testData.hdcpVer)

			typecDisplayInfoPattern := []*regexp.Regexp{displayInfoRe, hdcpVerRe}
			if err := usbutils.ExternalDisplayDetectionForLocal(ctx, numOfConnectedDisplays, typecDisplayInfoPattern); err != nil {
				s.Fatal("Failed to check for connected external display: ", err)
			}

			if testData.is4KDisplay {
				if err := typecutils.VerifyDisplay4KResolution(ctx); err != nil {
					s.Fatal("Failed to verify display 4K resolution: ", err)
				}
			}

			if err := cuj.SwitchWindowToDisplay(ctx, tconn, kb, isExtDisplay)(ctx); err != nil {
				s.Fatal("Failed to set display in extended mode: ", err)
			}

			// Enable drm logs with 'echo 0xe > /sys/module/drm/parameters/debug'.
			setDrmCommand := "echo 0xe > /sys/module/drm/parameters/debug"
			if err := testexec.CommandContext(ctx, "sh", "-c", setDrmCommand).Run(); err != nil {
				s.Fatal("Failed to set drm: ", err)
			}
			unsetDrmCommand := "echo 0 > /sys/module/drm/parameters/debug"
			defer testexec.CommandContext(cleanupCtx, "sh", "-c", unsetDrmCommand).Run()

			// Capture drm logs from the command 'dmesg -w' and write into file.
			cmd := testexec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("dmesg -w >> %s &", tmpFilePath))
			if err := cmd.Start(); err != nil {
				s.Fatal("Failed to start dmesg: ", err)
			}
			defer cmd.Kill()
		}

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
		if err := videoConn.VerifyVideoPlayWithDuration(ctx, 11, cras, expectedAudioNode); err != nil {
			s.Fatal("Failed to play verify video or audio routing: ", err)
		}

		if testData.display != "" {
			// For display tests:
			// Check if HDCP is enabled message is coming or not,
			// from drm logs when video is played.
			// Execute: dmesg -w | grep hdcp.
			if err := testing.Poll(ctx, func(ctx context.Context) error {
				output, err := ioutil.ReadFile(tmpFilePath)
				if err != nil {
					return testing.PollBreak(errors.Wrapf(err, "failed to read %s drm log file", tmpFilePath))
				}
				if !strings.Contains(string(output), testData.drmLogMsg) {
					return errors.New("failed to verify drm logs")
				}
				return nil
			}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: 1 * time.Second}); err != nil {
				s.Fatal("Failed to verify HDCP is enabled message from drm logs: ", err)
			}

			// c3. For display TCs: Check modetest -c
			modetestOut, err := testexec.CommandContext(ctx, "modetest", "-c").Output()
			if err != nil {
				s.Fatal("Failed to run modetest: ", err)
			}
			if !strings.Contains(string(modetestOut), testData.typeVal) {
				s.Fatalf("Failed to find %s in modetest", testData.typeVal)
			}

			if testData.isTBTDevice {
				if err := videoConn.VerifyL0Response(ctx); err != nil {
					s.Fatal("Failed to verify L0 response: ", err)
				}
			}
		}

		// Switch video between full screen and default screen 5 times.
		const iterValue = 5
		if err := videoConn.FullScreenEntryExit(ctx, iterValue); err != nil {
			s.Fatal("Failed to switch video fillscreen: ", err)
		}

		// Take screenshot and check video area is blank to confirm HW DRM is working.
		if err := videoConn.VerifyVideoBlankScreen(ctx, s.OutDir(), isExtDisplay); err != nil {
			s.Fatal("Failed to verify video blank screen: ", err)
		}
		videoConn.Conn.CloseTarget(ctx)
	}
}
