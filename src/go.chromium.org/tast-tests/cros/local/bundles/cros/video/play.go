// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/video/play"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/graphics/expectations"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type playParams struct {
	fileName     string
	videoType    play.VideoType
	verifyMode   play.VerifyHWAcceleratorMode
	browserType  browser.Type
	unmutePlayer bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         Play,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks simple video playback in Chrome is working",
		Contacts: []string{
			"chromeos-gfx-video@google.com",
			"mcasas@chromium.org",
		},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video
		SoftwareDeps: []string{"chrome"},
		Data:         []string{"video.html", "playback.js"},
		Params: []testing.Param{{
			Name: "av1",
			Val: playParams{
				fileName:    "bear-320x240.av1.mp4",
				videoType:   play.NormalVideo,
				verifyMode:  play.NoVerifyHWAcceleratorUsed,
				browserType: browser.TypeAsh,
			},
			ExtraAttr: []string{"group:graphics", "graphics_video", "graphics_perbuild", "group:cq-medium"},
			ExtraData: []string{"bear-320x240.av1.mp4"},
			Fixture:   "chromeVideo",
		}, {
			Name: "h264",
			Val: playParams{
				fileName:    "bear-320x240.h264.mp4",
				videoType:   play.NormalVideo,
				verifyMode:  play.NoVerifyHWAcceleratorUsed,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild", "group:cq-medium"},
			ExtraData:         []string{"bear-320x240.h264.mp4"},
			ExtraSoftwareDeps: []string{"proprietary_codecs"},
			Fixture:           "chromeVideo",
		}, {
			Name: "vp8",
			Val: playParams{
				fileName:    "bear-320x240.vp8.webm",
				videoType:   play.NormalVideo,
				verifyMode:  play.NoVerifyHWAcceleratorUsed,
				browserType: browser.TypeAsh,
			},
			ExtraAttr: []string{"group:graphics", "graphics_video", "graphics_perbuild", "group:cq-medium"},
			ExtraData: []string{"bear-320x240.vp8.webm"},
			Fixture:   "chromeVideo",
		}, {
			Name: "vp9",
			Val: playParams{
				fileName:    "bear-320x240.vp9.webm",
				videoType:   play.NormalVideo,
				verifyMode:  play.NoVerifyHWAcceleratorUsed,
				browserType: browser.TypeAsh,
			},
			ExtraAttr: []string{"group:graphics", "graphics_video", "graphics_perbuild", "group:cq-medium"},
			ExtraData: []string{"bear-320x240.vp9.webm"},
			Fixture:   "chromeVideo",
		}, {
			Name: "vp9_hdr",
			Val: playParams{
				fileName:    "peru.8k.cut.hdr.vp9.webm",
				videoType:   play.NormalVideo,
				verifyMode:  play.NoVerifyHWAcceleratorUsed,
				browserType: browser.TypeAsh,
			},
			ExtraAttr: []string{"group:graphics", "graphics_video", "graphics_nightly"},
			ExtraData: []string{"peru.8k.cut.hdr.vp9.webm"},
			Fixture:   "chromeVideoWithHDRScreen",
		}, {
			Name: "av1_sw",
			Val: playParams{
				fileName:    "bear-320x240.av1.mp4",
				videoType:   play.NormalVideo,
				verifyMode:  play.VerifyNoHWAcceleratorUsed,
				browserType: browser.TypeAsh,
			},
			ExtraAttr: []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData: []string{"bear-320x240.av1.mp4"},
			Fixture:   "chromeVideoWithSWDecoding",
		}, {
			Name: "h264_sw",
			Val: playParams{
				fileName:    "bear-320x240.h264.mp4",
				videoType:   play.NormalVideo,
				verifyMode:  play.VerifyNoHWAcceleratorUsed,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"bear-320x240.h264.mp4"},
			ExtraSoftwareDeps: []string{"proprietary_codecs"},
			Fixture:           "chromeVideoWithSWDecoding",
		}, {
			Name: "vp8_sw",
			Val: playParams{
				fileName:    "bear-320x240.vp8.webm",
				videoType:   play.NormalVideo,
				verifyMode:  play.VerifyNoHWAcceleratorUsed,
				browserType: browser.TypeAsh,
			},
			ExtraAttr: []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData: []string{"bear-320x240.vp8.webm"},
			Fixture:   "chromeVideoWithSWDecoding",
		}, {
			Name: "vp9_sw",
			Val: playParams{
				fileName:    "bear-320x240.vp9.webm",
				videoType:   play.NormalVideo,
				verifyMode:  play.VerifyNoHWAcceleratorUsed,
				browserType: browser.TypeAsh,
			},
			ExtraAttr: []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData: []string{"bear-320x240.vp9.webm"},
			Fixture:   "chromeVideoWithSWDecoding",
		}, {
			Name: "vp9_2_sw",
			Val: playParams{
				fileName:    "bear-320x240.vp9.2.webm",
				videoType:   play.NormalVideo,
				verifyMode:  play.VerifyNoHWAcceleratorUsed,
				browserType: browser.TypeAsh,
			},
			ExtraAttr: []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData: []string{"bear-320x240.vp9.2.webm"},
			Fixture:   "chromeVideoWithSWDecoding",
		}, {
			Name: "vp9_sw_hdr",
			Val: playParams{
				fileName:    "peru.8k.cut.hdr.vp9.webm",
				videoType:   play.NormalVideo,
				verifyMode:  play.VerifyNoHWAcceleratorUsed,
				browserType: browser.TypeAsh,
			},
			ExtraAttr: []string{"group:graphics", "graphics_video", "graphics_nightly"},
			ExtraData: []string{"peru.8k.cut.hdr.vp9.webm"},
			Fixture:   "chromeVideoWithSWDecodingAndHDRScreen",
		}, {
			Name: "av1_hw",
			Val: playParams{
				fileName:    "bear-320x240.av1.mp4",
				videoType:   play.NormalVideo,
				verifyMode:  play.VerifyHWAcceleratorUsed,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"bear-320x240.av1.mp4"},
			ExtraSoftwareDeps: []string{caps.HWDecodeAV1},
			Fixture:           "chromeVideo",
		}, {
			Name: "av1_hw_odd_dimension",
			Val: playParams{
				fileName:    "bear-321x241.av1.mp4",
				videoType:   play.NormalVideo,
				verifyMode:  play.VerifyHWAcceleratorUsed,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"bear-321x241.av1.mp4"},
			ExtraSoftwareDeps: []string{caps.HWDecodeAV1},
			Fixture:           "chromeVideo",
		}, {
			Name: "h264_hw",
			Val: playParams{
				fileName:    "bear-320x240.h264.mp4",
				videoType:   play.NormalVideo,
				verifyMode:  play.VerifyHWAcceleratorUsed,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"bear-320x240.h264.mp4"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideo",
		}, {
			Name: "h264_hw_lacros",
			Val: playParams{
				fileName:    "bear-320x240.h264.mp4",
				videoType:   play.NormalVideo,
				verifyMode:  play.VerifyHWAcceleratorUsed,
				browserType: browser.TypeLacros,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"bear-320x240.h264.mp4"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs", "lacros"},
			Fixture:           "chromeVideoLacros",
		}, {
			Name: "vp8_hw",
			Val: playParams{
				fileName:    "bear-320x240.vp8.webm",
				videoType:   play.NormalVideo,
				verifyMode:  play.VerifyHWAcceleratorUsed,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"bear-320x240.vp8.webm"},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP8},
			Fixture:           "chromeVideo",
		}, {
			Name: "vp8_hw_odd_dimension",
			Val: playParams{
				fileName:    "bear-321x241.vp8.webm",
				videoType:   play.NormalVideo,
				verifyMode:  play.VerifyHWAcceleratorUsed,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"bear-321x241.vp8.webm"},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP8},
			Fixture:           "chromeVideo",
		}, {
			Name: "vp9_hw",
			Val: playParams{
				fileName:    "bear-320x240.vp9.webm",
				videoType:   play.NormalVideo,
				verifyMode:  play.VerifyHWAcceleratorUsed,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"bear-320x240.vp9.webm"},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9},
			Fixture:           "chromeVideo",
		}, {
			Name: "vp9_hw_odd_dimension",
			Val: playParams{
				fileName:    "bear-321x241.vp9.webm",
				videoType:   play.NormalVideo,
				verifyMode:  play.VerifyHWAcceleratorUsed,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"bear-321x241.vp9.webm"},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9},
			Fixture:           "chromeVideo",
		}, {
			Name: "vp9_hw_lacros",
			Val: playParams{
				fileName:    "bear-320x240.vp9.webm",
				videoType:   play.NormalVideo,
				verifyMode:  play.VerifyHWAcceleratorUsed,
				browserType: browser.TypeLacros,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"bear-320x240.vp9.webm"},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9, "lacros"},
			Fixture:           "chromeVideoLacros",
		}, {
			Name: "vp9_2_hw",
			Val: playParams{
				fileName:    "bear-320x240.vp9.2.webm",
				videoType:   play.NormalVideo,
				verifyMode:  play.VerifyHWAcceleratorUsed,
				browserType: browser.TypeAsh,
			},
			ExtraAttr: []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData: []string{"bear-320x240.vp9.2.webm"},
			// VP9 Profile 2 is only supported by the direct Video Decoder.
			ExtraSoftwareDeps: []string{"video_decoder_direct", caps.HWDecodeVP9_2},
			Fixture:           "chromeVideo",
		}, {
			Name: "vp9_2_hw_inpvd",
			Val: playParams{
				fileName:    "bear-320x240.vp9.2.webm",
				videoType:   play.NormalVideo,
				verifyMode:  play.VerifyHWAcceleratorUsed,
				browserType: browser.TypeAsh,
			},
			ExtraAttr: []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData: []string{"bear-320x240.vp9.2.webm"},
			// VP9 Profile 2 is only supported by the direct Video Decoder.
			ExtraSoftwareDeps: []string{"video_decoder_direct", caps.HWDecodeVP9_2},
			Fixture:           "chromeVideoINPVD",
		}, {
			Name: "vp9_hw_hdr",
			Val: playParams{
				fileName:    "peru.8k.cut.hdr.vp9.webm",
				videoType:   play.NormalVideo,
				verifyMode:  play.VerifyHWAcceleratorUsed,
				browserType: browser.TypeAsh,
			},
			ExtraAttr: []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData: []string{"peru.8k.cut.hdr.vp9.webm"},
			// TODO(crbug.com/1057870): filter this by Intel SoC generation: KBL+. For now, kohaku will do.
			ExtraHardwareDeps: hwdep.D(hwdep.Model("kohaku")),
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9_2},
			Fixture:           "chromeVideoWithHDRScreen",
		}, {
			Name: "hevc_hw",
			Val: playParams{
				fileName:    "bear-320x240.hevc.mp4",
				videoType:   play.NormalVideo,
				verifyMode:  play.VerifyHWAcceleratorUsed,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild", "group:cq-medium"},
			ExtraData:         []string{"bear-320x240.hevc.mp4"},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsHEVCVideoDecodingInChrome()),
			ExtraSoftwareDeps: []string{caps.HWDecodeHEVC, "proprietary_codecs"},
			Fixture:           "chromeVideo",
		}, {
			Name: "hevc10_hw",
			Val: playParams{
				fileName:    "bear-320x240.hevc10.mp4",
				videoType:   play.NormalVideo,
				verifyMode:  play.VerifyHWAcceleratorUsed,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"bear-320x240.hevc10.mp4"},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsHEVCVideoDecodingInChrome()),
			ExtraSoftwareDeps: []string{caps.HWDecodeHEVC10BPP, "proprietary_codecs"},
			Fixture:           "chromeVideo",
		}, {
			Name: "h264_hw_mse",
			Val: playParams{
				fileName:    "bear-320x240.h264.mpd",
				videoType:   play.MSEVideo,
				verifyMode:  play.VerifyHWAcceleratorUsed,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.MSEDataFiles(), "bear-320x240-video-only.h264.mp4", "bear-320x240-audio-only.aac.mp4", "bear-320x240.h264.mpd"),
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideo",
		}, {
			Name: "vp8_hw_mse",
			Val: playParams{
				fileName:    "bear-320x240.vp8.mpd",
				videoType:   play.MSEVideo,
				verifyMode:  play.VerifyHWAcceleratorUsed,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.MSEDataFiles(), "bear-320x240-video-only.vp8.webm", "bear-320x240-audio-only.vorbis.webm", "bear-320x240.vp8.mpd"),
			ExtraSoftwareDeps: []string{caps.HWDecodeVP8},
			Fixture:           "chromeVideo",
		}, {
			Name: "vp9_hw_mse",
			Val: playParams{
				fileName:    "bear-320x240.vp9.mpd",
				videoType:   play.MSEVideo,
				verifyMode:  play.VerifyHWAcceleratorUsed,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.MSEDataFiles(), "bear-320x240-video-only.vp9.webm", "bear-320x240-audio-only.opus.webm", "bear-320x240.vp9.mpd"),
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9},
			Fixture:           "chromeVideo",
		}, {
			Name: "hevc_hw_mse",
			Val: playParams{
				fileName:    "bear-320x240.hevc.mpd",
				videoType:   play.MSEVideo,
				verifyMode:  play.VerifyHWAcceleratorUsed,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         append(play.MSEDataFiles(), "bear-320x240-video-only.hevc.mp4", "bear-320x240-audio-only.aac.mp4", "bear-320x240.hevc.mpd"),
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsHEVCVideoDecodingInChrome()),
			ExtraSoftwareDeps: []string{caps.HWDecodeHEVC, "proprietary_codecs"},
			Fixture:           "chromeVideo",
		}, {
			Name: "av1_guest",
			Val: playParams{
				fileName:    "bear-320x240.av1.mp4",
				videoType:   play.NormalVideo,
				verifyMode:  play.NoVerifyHWAcceleratorUsed,
				browserType: browser.TypeAsh,
			},
			ExtraAttr: []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData: []string{"bear-320x240.av1.mp4"},
			Fixture:   "chromeVideoWithGuestLogin",
		}, {
			Name: "h264_guest",
			Val: playParams{
				fileName:    "bear-320x240.h264.mp4",
				videoType:   play.NormalVideo,
				verifyMode:  play.NoVerifyHWAcceleratorUsed,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"bear-320x240.h264.mp4"},
			ExtraSoftwareDeps: []string{"proprietary_codecs"},
			Fixture:           "chromeVideoWithGuestLogin",
		}, {
			Name: "vp8_guest",
			Val: playParams{
				fileName:    "bear-320x240.vp8.webm",
				videoType:   play.NormalVideo,
				verifyMode:  play.NoVerifyHWAcceleratorUsed,
				browserType: browser.TypeAsh,
			},
			ExtraAttr: []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData: []string{"bear-320x240.vp8.webm"},
			Fixture:   "chromeVideoWithGuestLogin",
		}, {
			Name: "vp9_guest",
			Val: playParams{
				fileName:    "bear-320x240.vp9.webm",
				videoType:   play.NormalVideo,
				verifyMode:  play.NoVerifyHWAcceleratorUsed,
				browserType: browser.TypeAsh,
			},
			ExtraAttr: []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData: []string{"bear-320x240.vp9.webm"},
			Fixture:   "chromeVideoWithGuestLogin",
		}, {
			Name: "av1_unmuted",
			Val: playParams{
				fileName:     "bear-320x240.av1.mp4",
				videoType:    play.NormalVideo,
				verifyMode:   play.NoVerifyHWAcceleratorUsed,
				browserType:  browser.TypeAsh,
				unmutePlayer: true,
			},
			ExtraAttr: []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData: []string{"bear-320x240.av1.mp4"},
			Fixture:   "chromeVideo",
		}, {
			Name: "h264_unmuted",
			Val: playParams{
				fileName:     "bear-320x240.h264.mp4",
				videoType:    play.NormalVideo,
				verifyMode:   play.NoVerifyHWAcceleratorUsed,
				browserType:  browser.TypeAsh,
				unmutePlayer: true,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"bear-320x240.h264.mp4"},
			ExtraSoftwareDeps: []string{"proprietary_codecs"},
			Fixture:           "chromeVideo",
		}, {
			Name: "vp8_unmuted",
			Val: playParams{
				fileName:     "bear-320x240.vp8.webm",
				videoType:    play.NormalVideo,
				verifyMode:   play.NoVerifyHWAcceleratorUsed,
				browserType:  browser.TypeAsh,
				unmutePlayer: true,
			},
			ExtraAttr: []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData: []string{"bear-320x240.vp8.webm"},
			Fixture:   "chromeVideo",
		}, {
			Name: "vp9_unmuted",
			Val: playParams{
				fileName:     "bear-320x240.vp9.webm",
				videoType:    play.NormalVideo,
				verifyMode:   play.NoVerifyHWAcceleratorUsed,
				browserType:  browser.TypeAsh,
				unmutePlayer: true,
			},
			ExtraAttr: []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData: []string{"bear-320x240.vp9.webm"},
			Fixture:   "chromeVideo",
		}, {
			Name: "h264_hw_v4l2_flat",
			Val: playParams{
				fileName:    "bear-320x240.h264.mp4",
				videoType:   play.NormalVideo,
				verifyMode:  play.VerifyHWAcceleratorUsed,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"bear-320x240.h264.mp4"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs", "v4l2_codec"},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsV4L2StatefulVideoDecoding()), // E.g. MT8173 Hana and QC SC7180 Trogdor.
			Fixture:           "chromeVideoWithV4L2FlatDecoder",
		}, {
			Name: "vp8_hw_v4l2_flat",
			Val: playParams{
				fileName:    "bear-320x240.vp8.webm",
				videoType:   play.NormalVideo,
				verifyMode:  play.VerifyHWAcceleratorUsed,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"bear-320x240.vp8.webm"},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP8, "v4l2_codec"},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsV4L2StatefulVideoDecoding()), // E.g. MT8173 Hana and QC SC7180 Trogdor.
			Fixture:           "chromeVideoWithV4L2FlatDecoder",
		}, {
			Name: "vp9_hw_v4l2_flat",
			Val: playParams{
				fileName:    "bear-320x240.vp9.webm",
				videoType:   play.NormalVideo,
				verifyMode:  play.VerifyHWAcceleratorUsed,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"bear-320x240.vp9.webm"},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9, "v4l2_codec"},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsV4L2StatefulVideoDecoding()), // E.g. MT8173 Hana and QC SC7180 Trogdor.
			Fixture:           "chromeVideoWithV4L2FlatDecoder",
		}},
	})
}

// Play plays a given file in Chrome and verifies that the playback happens
// correctly; if verifyMode says so, it verifies if playback uses hardware
// acceleration.
// If videoType is NormalVideo, a simple <video> player is instantiated with the
// input filename, whereas if it's MSEVideo,then we try to feed the media files
// via a SourceBuffer (using MSE, the Media Source Extensions protocol, and a
// DASH MPD file).
func Play(ctx context.Context, s *testing.State) {
	expectation, err := expectations.GetTestExpectation(ctx, s.TestName())
	if err != nil {
		s.Fatal("Failed to load test expectation: ", err)
	}
	testOpt := s.Param().(playParams)

	cr, l, cs, err := lacros.Setup(ctx, s.FixtValue(), testOpt.browserType)
	if err != nil {
		s.Fatal("Failed to initialize test: ", err)
	}
	defer lacros.CloseLacros(ctx, l)

	if err := play.TestPlay(ctx, s, cs, cr, testOpt.fileName, testOpt.videoType, testOpt.verifyMode, testOpt.unmutePlayer); err != nil {
		if expErr := expectation.ReportError("test failed: ", err); expErr != nil {
			s.Fatal("Unexpected error: ", expErr)
		}
	}
}
