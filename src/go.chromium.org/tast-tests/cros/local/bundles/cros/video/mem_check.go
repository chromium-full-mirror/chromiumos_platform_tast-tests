// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/video/play"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast-tests/cros/local/media/pre"
	"go.chromium.org/tast-tests/cros/local/mountns"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type memCheckParams struct {
	fileName    string
	sizes       []graphics.Size
	videoType   play.VideoType
	browserType browser.Type
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         MemCheck,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks video playback in Chrome has no leaks",
		Contacts: []string{
			"chromeos-gfx-video@google.com",
			"mcasas@chromium.org",
		},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video
		HardwareDeps: hwdep.D(hwdep.SupportsNV12Overlays()),
		SoftwareDeps: []string{"chrome"},
		Data:         []string{"video.html", "playback.js"},
		Params: []testing.Param{{
			Name:              "av1_hw",
			Val:               memCheckParams{fileName: "720_av1.mp4", sizes: []graphics.Size{{Width: 1280, Height: 720}}, videoType: play.NormalVideo, browserType: browser.TypeAsh},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_nightly"},
			ExtraData:         []string{"720_av1.mp4"},
			ExtraSoftwareDeps: []string{"video_overlays", caps.HWDecodeAV1},
			Fixture:           "chromeVideoWithGuestLogin",
			Timeout:           10 * time.Minute,
		}, {
			Name:              "h264_hw",
			Val:               memCheckParams{fileName: "720_h264.mp4", sizes: []graphics.Size{{Width: 1280, Height: 720}}, videoType: play.NormalVideo, browserType: browser.TypeAsh},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_nightly"},
			ExtraData:         []string{"720_h264.mp4"},
			ExtraSoftwareDeps: []string{"video_overlays", caps.HWDecodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideoWithGuestLogin",
			Timeout:           10 * time.Minute,
		}, {
			Name:              "h264_hw_gtfo",
			Val:               memCheckParams{fileName: "720_h264.mp4", sizes: []graphics.Size{{Width: 1280, Height: 720}}, videoType: play.NormalVideo, browserType: browser.TypeAsh},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_nightly"},
			ExtraData:         []string{"720_h264.mp4"},
			ExtraSoftwareDeps: []string{"video_overlays", caps.HWDecodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideoGTFOWithGuestLogin",
			Timeout:           10 * time.Minute,
		}, {
			Name:              "h264_hw_inpvd",
			Val:               memCheckParams{fileName: "720_h264.mp4", sizes: []graphics.Size{{Width: 1280, Height: 720}}, videoType: play.NormalVideo, browserType: browser.TypeAsh},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_nightly"},
			ExtraData:         []string{"720_h264.mp4"},
			ExtraSoftwareDeps: []string{"video_overlays", caps.HWDecodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideoINPVDWithGuestLogin",
			Timeout:           10 * time.Minute,
		}, {
			Name:              "hevc_hw",
			Val:               memCheckParams{fileName: "720_hevc.mp4", sizes: []graphics.Size{{Width: 1280, Height: 720}}, videoType: play.NormalVideo, browserType: browser.TypeAsh},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_nightly"},
			ExtraData:         []string{"720_hevc.mp4"},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsHEVCVideoDecodingInChrome()),
			ExtraSoftwareDeps: []string{"video_overlays", caps.HWDecodeHEVC, "proprietary_codecs"},
			Fixture:           "chromeVideoWithGuestLogin",
			Timeout:           10 * time.Minute,
		}, {
			Name:              "vp8_hw",
			Val:               memCheckParams{fileName: "720_vp8.webm", sizes: []graphics.Size{{Width: 1280, Height: 720}}, videoType: play.NormalVideo, browserType: browser.TypeAsh},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_nightly"},
			ExtraData:         []string{"720_vp8.webm"},
			ExtraSoftwareDeps: []string{"video_overlays", caps.HWDecodeVP8},
			Fixture:           "chromeVideoWithGuestLogin",
			Timeout:           10 * time.Minute,
		}, {
			Name:              "vp9_hw",
			Val:               memCheckParams{fileName: "720_vp9.webm", sizes: []graphics.Size{{Width: 1280, Height: 720}}, videoType: play.NormalVideo, browserType: browser.TypeAsh},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_nightly"},
			ExtraData:         []string{"720_vp9.webm"},
			ExtraSoftwareDeps: []string{"video_overlays", caps.HWDecodeVP9},
			Fixture:           "chromeVideoWithGuestLogin",
			Timeout:           10 * time.Minute,
		}, {
			Name:              "av1_hw_switch",
			Val:               memCheckParams{fileName: "dash_smpte_av1.mp4.mpd", sizes: []graphics.Size{{Width: 256, Height: 144}, {Width: 426, Height: 240}}, videoType: play.MSEVideo, browserType: browser.TypeAsh},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_nightly"},
			ExtraData:         append(play.MSEDataFiles(), "dash_smpte_av1.mp4.mpd", "dash_smpte_144.av1.mp4", "dash_smpte_240.av1.mp4"),
			ExtraSoftwareDeps: []string{"video_overlays", caps.HWDecodeAV1},
			Fixture:           "chromeVideoWithGuestLogin",
			Timeout:           10 * time.Minute,
		}, {
			Name:              "h264_hw_switch",
			Val:               memCheckParams{fileName: "cars_dash_mp4.mpd", sizes: []graphics.Size{{Width: 256, Height: 144}, {Width: 426, Height: 240}}, videoType: play.MSEVideo, browserType: browser.TypeAsh},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_weekly"},
			ExtraData:         append(play.MSEDataFiles(), "cars_dash_mp4.mpd", "cars_144_h264.mp4", "cars_240_h264.mp4"),
			ExtraSoftwareDeps: []string{"video_overlays", caps.HWDecodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideoWithGuestLogin",
			Timeout:           10 * time.Minute,
		}, {
			Name:              "hevc_hw_switch",
			Val:               memCheckParams{fileName: "cars_dash_hevc.mpd", sizes: []graphics.Size{{Width: 256, Height: 144}, {Width: 426, Height: 240}}, videoType: play.MSEVideo, browserType: browser.TypeAsh},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_weekly"},
			ExtraData:         append(play.MSEDataFiles(), "cars_dash_hevc.mpd", "cars_144_hevc.mp4", "cars_240_hevc.mp4"),
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsHEVCVideoDecodingInChrome()),
			ExtraSoftwareDeps: []string{"video_overlays", caps.HWDecodeHEVC, "proprietary_codecs"},
			Fixture:           "chromeVideoWithGuestLogin",
			Timeout:           10 * time.Minute,
		}, {
			Name:              "av1_hw_lacros",
			Val:               memCheckParams{fileName: "720_av1.mp4", sizes: []graphics.Size{{Width: 1280, Height: 720}}, videoType: play.NormalVideo, browserType: browser.TypeLacros},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_nightly"},
			ExtraData:         []string{"720_av1.mp4"},
			ExtraSoftwareDeps: []string{"video_overlays", caps.HWDecodeAV1, "lacros"},
			Fixture:           pre.ChromeVideoLacrosFixture(pre.GuestLogin),
			Timeout:           10 * time.Minute,
		}, {
			Name:              "h264_hw_lacros",
			Val:               memCheckParams{fileName: "720_h264.mp4", sizes: []graphics.Size{{Width: 1280, Height: 720}}, videoType: play.NormalVideo, browserType: browser.TypeLacros},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_nightly"},
			ExtraData:         []string{"720_h264.mp4"},
			ExtraSoftwareDeps: []string{"video_overlays", caps.HWDecodeH264, "proprietary_codecs", "lacros"},
			Fixture:           pre.ChromeVideoLacrosFixture(pre.GuestLogin),
			Timeout:           10 * time.Minute,
		}, {
			Name:              "hevc_hw_lacros",
			Val:               memCheckParams{fileName: "720_hevc.mp4", sizes: []graphics.Size{{Width: 1280, Height: 720}}, videoType: play.NormalVideo, browserType: browser.TypeLacros},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_nightly"},
			ExtraData:         []string{"720_hevc.mp4"},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsHEVCVideoDecodingInChrome()),
			ExtraSoftwareDeps: []string{"video_overlays", caps.HWDecodeHEVC, "proprietary_codecs", "lacros"},
			Fixture:           pre.ChromeVideoLacrosFixture(pre.GuestLogin),
			Timeout:           10 * time.Minute,
		}, {
			Name:              "vp8_hw_lacros",
			Val:               memCheckParams{fileName: "720_vp8.webm", sizes: []graphics.Size{{Width: 1280, Height: 720}}, videoType: play.NormalVideo, browserType: browser.TypeLacros},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_nightly"},
			ExtraData:         []string{"720_vp8.webm"},
			ExtraSoftwareDeps: []string{"video_overlays", caps.HWDecodeVP8, "lacros"},
			Fixture:           pre.ChromeVideoLacrosFixture(pre.GuestLogin),
			Timeout:           10 * time.Minute,
		}, {
			Name:              "vp9_hw_lacros",
			Val:               memCheckParams{fileName: "720_vp9.webm", sizes: []graphics.Size{{Width: 1280, Height: 720}}, videoType: play.NormalVideo, browserType: browser.TypeLacros},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_nightly"},
			ExtraData:         []string{"720_vp9.webm"},
			ExtraSoftwareDeps: []string{"video_overlays", caps.HWDecodeVP9, "lacros"},
			Fixture:           pre.ChromeVideoLacrosFixture(pre.GuestLogin),
			Timeout:           10 * time.Minute,
		}, {
			Name:              "h264_hw_lacros_gtfo",
			Val:               memCheckParams{fileName: "720_h264.mp4", sizes: []graphics.Size{{Width: 1280, Height: 720}}, videoType: play.NormalVideo, browserType: browser.TypeLacros},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_nightly"},
			ExtraData:         []string{"720_h264.mp4"},
			ExtraSoftwareDeps: []string{"video_overlays", caps.HWDecodeH264, "proprietary_codecs", "lacros"},
			Fixture:           pre.ChromeVideoLacrosFixture(pre.GuestLogin, pre.GTFO),
			Timeout:           10 * time.Minute,
		}},
	})
}

func MemCheck(ctx context.Context, s *testing.State) {
	// In order for the lacros variants to work correctly, we need to
	// run the test body in the user mount namespace. See b/244513681.
	if err := mountns.WithUserSessionMountNS(ctx, func(ctx context.Context) error {
		memCheck(ctx, s)
		return nil
	}); err != nil {
		s.Fatal("Failed to run test in correct mount namespace: ", err)
	}
}

// memCheck plays a given fileName in Chrome and verifies there are no graphics
// memory leaks by comparing its usage before, during and after.
func memCheck(ctx context.Context, s *testing.State) {
	testOpt := s.Param().(memCheckParams)
	const unmutePlayer = false

	cr, l, cs, err := lacros.Setup(ctx, s.FixtValue(), testOpt.browserType)
	if err != nil {
		s.Fatal("Failed to initialize test: ", err)
	}
	defer lacros.CloseLacros(ctx, l)

	testPlay := func() error {
		return play.TestPlay(ctx, s, cs, cr, testOpt.fileName, testOpt.videoType, play.VerifyHWAcceleratorUsed, unmutePlayer)
	}

	backend, err := graphics.GetBackend()
	if err != nil {
		s.Fatal("Error getting the graphics backend: ", err)
	}
	if err := graphics.VerifyGraphicsMemory(ctx, testPlay, backend, testOpt.sizes); err != nil {
		s.Fatal("Test failed: ", err)
	}
}
