// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"context"
	"net/http"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/video/play"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/media/pre"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// seekTest is used to describe the config used to run each Seek test.
type seekTest struct {
	filename    string // File name to play back.
	numSeeks    int    // Amount of times to seek into the <video>.
	browserType browser.Type
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         Seek,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verifies that seeking works in Chrome, either with or without resolution changes",
		Contacts: []string{
			"chromeos-gfx-video@google.com",
			"mcasas@chromium.org",
		},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video
		SoftwareDeps: []string{"chrome"},
		Data:         []string{"video.html", "playback.js"},
		Params: []testing.Param{{
			Name: "av1",
			Val: seekTest{
				filename:    "720_av1.mp4",
				numSeeks:    25,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"720_av1.mp4"},
			ExtraSoftwareDeps: []string{caps.HWDecodeAV1},
			Fixture:           "chromeVideo",
		}, {
			Name: "h264",
			Val: seekTest{
				filename:    "720_h264.mp4",
				numSeeks:    25,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"720_h264.mp4"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideo",
		}, {
			Name: "h264_inpvd",
			Val: seekTest{
				filename:    "720_h264.mp4",
				numSeeks:    25,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"720_h264.mp4"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideoINPVD",
		}, {
			Name: "h264_lacros",
			Val: seekTest{
				filename:    "720_h264.mp4",
				numSeeks:    25,
				browserType: browser.TypeLacros,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"720_h264.mp4"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs", "lacros"},
			Fixture:           pre.ChromeVideoLacrosFixture(pre.FakeMediaStreamUI),
		}, {
			Name: "h264_lacros_gtfo",
			Val: seekTest{
				filename:    "720_h264.mp4",
				numSeeks:    25,
				browserType: browser.TypeLacros,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"720_h264.mp4"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs", "lacros"},
			Fixture:           pre.ChromeVideoLacrosFixture(pre.FakeMediaStreamUI, pre.GTFO),
		}, {
			Name: "hevc",
			Val: seekTest{
				filename:    "720_hevc.mp4",
				numSeeks:    25,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"720_hevc.mp4"},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsHEVCVideoDecodingInChrome()),
			ExtraSoftwareDeps: []string{caps.HWDecodeHEVC, "proprietary_codecs"},
			Fixture:           "chromeVideo",
		}, {
			Name: "vp8",
			Val: seekTest{
				filename:    "720_vp8.webm",
				numSeeks:    25,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"720_vp8.webm"},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP8},
			Fixture:           "chromeVideo",
		}, {
			Name: "vp9",
			Val: seekTest{
				filename:    "720_vp9.webm",
				numSeeks:    25,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"720_vp9.webm"},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9},
			Fixture:           "chromeVideo",
		}, {
			Name: "vp9_lacros",
			Val: seekTest{
				filename:    "720_vp9.webm",
				numSeeks:    25,
				browserType: browser.TypeLacros,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"720_vp9.webm"},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9, "lacros"},
			Fixture:           pre.ChromeVideoLacrosFixture(pre.FakeMediaStreamUI),
		}, {
			Name: "switch_av1",
			Val: seekTest{
				filename:    "smpte_bars_resolution_ladder.av1.webm",
				numSeeks:    25,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"smpte_bars_resolution_ladder.av1.webm"},
			ExtraSoftwareDeps: []string{caps.HWDecodeAV1},
			Fixture:           "chromeVideo",
		}, {
			Name: "switch_h264",
			Val: seekTest{
				filename:    "smpte_bars_resolution_ladder.h264.mp4",
				numSeeks:    25,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"smpte_bars_resolution_ladder.h264.mp4"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideo",
		}, {
			Name: "switch_h264_inpvd",
			Val: seekTest{
				filename:    "smpte_bars_resolution_ladder.h264.mp4",
				numSeeks:    25,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"smpte_bars_resolution_ladder.h264.mp4"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideoINPVD",
		}, {
			Name: "switch_hevc",
			Val: seekTest{
				filename:    "smpte_bars_resolution_ladder.hevc.mp4",
				numSeeks:    25,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"smpte_bars_resolution_ladder.hevc.mp4"},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsHEVCVideoDecodingInChrome()),
			ExtraSoftwareDeps: []string{caps.HWDecodeHEVC, "proprietary_codecs"},
			Fixture:           "chromeVideo",
		}, {
			Name: "switch_vp8",
			Val: seekTest{
				filename:    "smpte_bars_resolution_ladder.vp8.webm",
				numSeeks:    25,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"smpte_bars_resolution_ladder.vp8.webm"},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP8},
			Fixture:           "chromeVideo",
		}, {
			Name: "switch_vp9",
			Val: seekTest{
				filename:    "smpte_bars_resolution_ladder.vp9.webm",
				numSeeks:    25,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"smpte_bars_resolution_ladder.vp9.webm"},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9},
			Fixture:           "chromeVideo",
		}, {
			Name: "stress_av1",
			Val: seekTest{
				filename:    "720_av1.mp4",
				numSeeks:    1000,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_weekly"},
			ExtraData:         []string{"720_av1.mp4"},
			ExtraSoftwareDeps: []string{caps.HWDecodeAV1},
			Timeout:           20 * time.Minute,
			Fixture:           "chromeVideo",
		}, {
			Name: "stress_vp8",
			Val: seekTest{
				filename:    "720_vp8.webm",
				numSeeks:    1000,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_weekly"},
			ExtraData:         []string{"720_vp8.webm"},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP8},
			Timeout:           20 * time.Minute,
			Fixture:           "chromeVideo",
		}, {
			Name: "stress_vp9",
			Val: seekTest{
				filename:    "720_vp9.webm",
				numSeeks:    1000,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_weekly"},
			ExtraData:         []string{"720_vp9.webm"},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9},
			Timeout:           20 * time.Minute,
			Fixture:           "chromeVideo",
		}, {
			Name: "stress_h264",
			Val: seekTest{
				filename:    "720_h264.mp4",
				numSeeks:    1000,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_weekly"},
			ExtraData:         []string{"720_h264.mp4"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			Timeout:           20 * time.Minute,
			Fixture:           "chromeVideo",
		}, {
			Name: "stress_hevc",
			Val: seekTest{
				filename:    "720_hevc.mp4",
				numSeeks:    1000,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_weekly"},
			ExtraData:         []string{"720_hevc.mp4"},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsHEVCVideoDecodingInChrome()),
			ExtraSoftwareDeps: []string{caps.HWDecodeHEVC, "proprietary_codecs"},
			Timeout:           20 * time.Minute,
			Fixture:           "chromeVideo",
		}, {Name: "av1_v4l2_flat",
			Val: seekTest{
				filename:    "720_av1.mp4",
				numSeeks:    25,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsV4L2FlatVideoDecoding()),
			ExtraData:         []string{"720_av1.mp4"},
			ExtraSoftwareDeps: []string{caps.HWDecodeAV1, "v4l2_codec"},
			Fixture:           "chromeVideoWithV4L2FlatDecoder",
		}, {
			Name: "h264_v4l2_flat",
			Val: seekTest{
				filename:    "720_h264.mp4",
				numSeeks:    25,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"720_h264.mp4"},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsV4L2FlatVideoDecoding()),
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs", "v4l2_codec"},
			Fixture:           "chromeVideoWithV4L2FlatDecoder",
		}, {
			Name: "hevc_v4l2_flat",
			Val: seekTest{
				filename:    "720_hevc.mp4",
				numSeeks:    25,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"720_hevc.mp4"},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsHEVCVideoDecodingInChrome(), hwdep.SupportsV4L2FlatVideoDecoding()),
			ExtraSoftwareDeps: []string{caps.HWDecodeHEVC, "proprietary_codecs", "v4l2_codec"},
			Fixture:           "chromeVideoWithV4L2FlatDecoder",
		}, {
			Name: "vp8_v4l2_flat",
			Val: seekTest{
				filename:    "720_vp8.webm",
				numSeeks:    25,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"720_vp8.webm"},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsV4L2FlatVideoDecoding()),
			ExtraSoftwareDeps: []string{caps.HWDecodeVP8, "v4l2_codec"},
			Fixture:           "chromeVideoWithV4L2FlatDecoder",
		}, {
			Name: "vp9_v4l2_flat",
			Val: seekTest{
				filename:    "720_vp9.webm",
				numSeeks:    25,
				browserType: browser.TypeAsh,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"720_vp9.webm"},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsV4L2FlatVideoDecoding()),
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9, "v4l2_codec"},
			Fixture:           "chromeVideoWithV4L2FlatDecoder",
		}},
	})
}

// Seek plays a file with Chrome and checks that it can safely be seeked into.
func Seek(ctx context.Context, s *testing.State) {
	testOpt := s.Param().(seekTest)

	_, l, cs, err := lacros.Setup(ctx, s.FixtValue(), testOpt.browserType)
	if err != nil {
		s.Fatal("Failed to initialize test: ", err)
	}
	defer lacros.CloseLacros(ctx, l)

	if err := play.TestSeek(ctx, http.FileServer(s.DataFileSystem()), cs, testOpt.filename, s.OutDir(), testOpt.numSeeks); err != nil {
		s.Fatal("TestSeek failed: ", err)
	}
}
