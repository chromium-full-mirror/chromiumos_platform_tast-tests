// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/video/play"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/media/pre"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type contentsParams struct {
	fileName    string
	refFileName string
	browserType browser.Type
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         Contents,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verifies that a screenshot of a full screen is valid",
		Contacts: []string{
			"chromeos-gfx-video@google.com",
			"andrescj@chromium.org",
		},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video
		// TODO(b/162437142): reenable on Zork when it does not hang forever.
		HardwareDeps: hwdep.D(hwdep.InternalDisplay(), hwdep.SkipGPUFamily("picasso")),
		SoftwareDeps: []string{"chrome"},
		Data:         []string{"video.html", "playback.js"},
		Params: []testing.Param{{
			Name: "h264_360p_hw",
			Val: contentsParams{
				fileName:    "still-colors-360p.h264.mp4",
				refFileName: "still-colors-360p.ref.png",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"still-colors-360p.h264.mp4", "still-colors-360p.ref.png"},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsNV12Overlays()),
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideo",
		}, {
			Name: "h264_360p_hw_inpvd",
			Val: contentsParams{
				fileName:    "still-colors-360p.h264.mp4",
				refFileName: "still-colors-360p.ref.png",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"still-colors-360p.h264.mp4", "still-colors-360p.ref.png"},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsNV12Overlays()),
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideoINPVD",
		}, {
			// TODO(andrescj): move to graphics_nightly after the test is stabilized.
			Name: "h264_360p_exotic_crop_hw",
			Val: contentsParams{
				fileName:    "still-colors-720x480-cropped-to-640x360.h264.mp4",
				refFileName: "still-colors-360p.ref.png",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"still-colors-720x480-cropped-to-640x360.h264.mp4", "still-colors-360p.ref.png"},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsNV12Overlays()),
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideo",
		}, {
			// TODO(andrescj): move to graphics_nightly after the test is stabilized.
			Name: "h264_360p_exotic_crop_hw_gtfo",
			Val: contentsParams{
				fileName:    "still-colors-720x480-cropped-to-640x360.h264.mp4",
				refFileName: "still-colors-360p.ref.png",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"still-colors-720x480-cropped-to-640x360.h264.mp4", "still-colors-360p.ref.png"},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsNV12Overlays()),
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideoGTFO",
		}, {
			// TODO(andrescj): move to graphics_nightly after the test is stabilized.
			Name: "h264_360p_exotic_crop_hw_lacros",
			Val: contentsParams{
				fileName:    "still-colors-720x480-cropped-to-640x360.h264.mp4",
				refFileName: "still-colors-360p.ref.png",
				browserType: browser.TypeLacros,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"still-colors-720x480-cropped-to-640x360.h264.mp4", "still-colors-360p.ref.png"},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsNV12Overlays()),
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs", "lacros"},
			Fixture:           pre.ChromeVideoLacrosFixture(pre.FakeMediaStreamUI),
		}, {
			// TODO(andrescj): move to graphics_nightly after the test is stabilized.
			Name: "h264_360p_exotic_crop_hw_lacros_gtfo",
			Val: contentsParams{
				fileName:    "still-colors-720x480-cropped-to-640x360.h264.mp4",
				refFileName: "still-colors-360p.ref.png",
				browserType: browser.TypeLacros,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"still-colors-720x480-cropped-to-640x360.h264.mp4", "still-colors-360p.ref.png"},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsNV12Overlays()),
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs", "lacros"},
			Fixture:           pre.ChromeVideoLacrosFixture(pre.FakeMediaStreamUI, pre.GTFO),
		}, {
			Name: "h264_480p_hw",
			Val: contentsParams{
				fileName:    "still-colors-480p.h264.mp4",
				refFileName: "still-colors-480p.ref.png",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"still-colors-480p.h264.mp4", "still-colors-480p.ref.png"},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsNV12Overlays()),
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideo",
		}, {
			Name: "h264_720p_hw",
			Val: contentsParams{
				fileName:    "still-colors-720p.h264.mp4",
				refFileName: "still-colors-720p.ref.png",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"still-colors-720p.h264.mp4", "still-colors-720p.ref.png"},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsNV12Overlays()),
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideo",
		}, {
			Name: "h264_1080p_hw",
			Val: contentsParams{
				fileName:    "still-colors-1080p.h264.mp4",
				refFileName: "still-colors-1080p.ref.png",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"still-colors-1080p.h264.mp4", "still-colors-1080p.ref.png"},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsNV12Overlays()),
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideo",
		}, {
			Name: "h264_360p_composited_hw",
			Val: contentsParams{
				fileName:    "still-colors-360p.h264.mp4",
				refFileName: "still-colors-360p.ref.png",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"still-colors-360p.h264.mp4", "still-colors-360p.ref.png"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			Fixture:           "chromeCompositedVideo",
		}, {
			// TODO(andrescj): move to graphics_nightly after the test is stabilized.
			Name: "h264_360p_exotic_crop_composited_hw",
			Val: contentsParams{
				fileName:    "still-colors-720x480-cropped-to-640x360.h264.mp4",
				refFileName: "still-colors-360p.ref.png",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"still-colors-720x480-cropped-to-640x360.h264.mp4", "still-colors-360p.ref.png"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			Fixture:           "chromeCompositedVideo",
		}, {
			// TODO(andrescj): move to graphics_nightly after the test is stabilized.
			Name: "h264_360p_exotic_crop_composited_hw_gtfo",
			Val: contentsParams{
				fileName:    "still-colors-720x480-cropped-to-640x360.h264.mp4",
				refFileName: "still-colors-360p.ref.png",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"still-colors-720x480-cropped-to-640x360.h264.mp4", "still-colors-360p.ref.png"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			Fixture:           "chromeCompositedVideoGTFO",
		}, {
			// TODO(andrescj): move to graphics_nightly after the test is stabilized.
			Name: "h264_360p_exotic_crop_ash_composited_hw_lacros",
			Val: contentsParams{
				fileName:    "still-colors-720x480-cropped-to-640x360.h264.mp4",
				refFileName: "still-colors-360p.ref.png",
				browserType: browser.TypeLacros,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"still-colors-720x480-cropped-to-640x360.h264.mp4", "still-colors-360p.ref.png"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs", "lacros"},
			Fixture:           pre.ChromeVideoLacrosFixture(pre.AshComposited),
		}, {
			// TODO(andrescj): move to graphics_nightly after the test is stabilized.
			Name: "h264_360p_exotic_crop_ash_composited_hw_lacros_gtfo",
			Val: contentsParams{
				fileName:    "still-colors-720x480-cropped-to-640x360.h264.mp4",
				refFileName: "still-colors-360p.ref.png",
				browserType: browser.TypeLacros,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"still-colors-720x480-cropped-to-640x360.h264.mp4", "still-colors-360p.ref.png"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs", "lacros"},
			Fixture:           pre.ChromeVideoLacrosFixture(pre.AshComposited, pre.GTFO),
		}, {
			// TODO(andrescj): move to graphics_nightly after the test is stabilized.
			Name: "h264_360p_exotic_crop_lacros_composited_hw_lacros",
			Val: contentsParams{
				fileName:    "still-colors-720x480-cropped-to-640x360.h264.mp4",
				refFileName: "still-colors-360p.ref.png",
				browserType: browser.TypeLacros,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"still-colors-720x480-cropped-to-640x360.h264.mp4", "still-colors-360p.ref.png"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs", "lacros"},
			Fixture:           pre.ChromeVideoLacrosFixture(pre.LacrosComposited),
		}, {
			// TODO(andrescj): move to graphics_nightly after the test is stabilized.
			Name: "h264_360p_exotic_crop_lacros_composited_hw_lacros_gtfo",
			Val: contentsParams{
				fileName:    "still-colors-720x480-cropped-to-640x360.h264.mp4",
				refFileName: "still-colors-360p.ref.png",
				browserType: browser.TypeLacros,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"still-colors-720x480-cropped-to-640x360.h264.mp4", "still-colors-360p.ref.png"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs", "lacros"},
			Fixture:           pre.ChromeVideoLacrosFixture(pre.LacrosComposited, pre.GTFO),
		}, {
			Name: "h264_480p_composited_hw",
			Val: contentsParams{
				fileName:    "still-colors-480p.h264.mp4",
				refFileName: "still-colors-480p.ref.png",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"still-colors-480p.h264.mp4", "still-colors-480p.ref.png"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			Fixture:           "chromeCompositedVideo",
		}, {
			Name: "h264_720p_composited_hw",
			Val: contentsParams{
				fileName:    "still-colors-720p.h264.mp4",
				refFileName: "still-colors-720p.ref.png",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"still-colors-720p.h264.mp4", "still-colors-720p.ref.png"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			Fixture:           "chromeCompositedVideo",
		}, {
			Name: "h264_1080p_composited_hw",
			Val: contentsParams{
				fileName:    "still-colors-1080p.h264.mp4",
				refFileName: "still-colors-1080p.ref.png",
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"still-colors-1080p.h264.mp4", "still-colors-1080p.ref.png"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			Fixture:           "chromeCompositedVideo",
		}},
		// TODO(andrescj): add tests for VP8 and VP9.
		// TODO(andrescj): for non-composited tests, check that overlays were used.
	})
}

// Contents starts playing a video, takes a screenshot, and checks a few interesting pixels.
func Contents(ctx context.Context, s *testing.State) {
	testOpt := s.Param().(contentsParams)

	cr, l, cs, err := lacros.Setup(ctx, s.FixtValue(), testOpt.browserType)
	if err != nil {
		s.Fatal("Failed to initialize test: ", err)
	}
	defer lacros.CloseLacros(ctx, l)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	if err := play.TestPlayAndScreenshot(ctx, s, tconn, cs, testOpt.fileName, testOpt.refFileName); err != nil {
		s.Fatal("TestPlayAndScreenshot failed: ", err)
	}
}
