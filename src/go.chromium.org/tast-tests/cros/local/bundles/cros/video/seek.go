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
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// seekTest is used to describe the config used to run each Seek test.
type seekTest struct {
	filename string // File name to play back.
	numSeeks int    // Amount of times to seek into the <video>.
}

func init() {
	testing.AddTest(&testing.Test{
		Func: Seek,
		Desc: "Verifies that seeking works in Chrome, either with or without resolution changes",
		Contacts: []string{
			"chromeos-gfx-video@google.com",
			"hiroh@chromium.org",
			"nhebert@chromium.org",
		},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video
		SoftwareDeps: []string{"chrome"},
		Data:         []string{"video.html", "playback.js"},
		Attr:         []string{"group:release-health", "release-health_gfx"},
		Params: []testing.Param{{
			Name: "av1",
			Val: seekTest{
				filename: "720_av1.mp4",
				numSeeks: 25,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"720_av1.mp4"},
			ExtraSoftwareDeps: []string{caps.HWDecodeAV1},
			Fixture:           "chromeVideo",
		}, {
			Name: "h264",
			Val: seekTest{
				filename: "720_h264.mp4",
				numSeeks: 25,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"720_h264.mp4"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideo",
		}, {
			Name: "h264_inpvd",
			Val: seekTest{
				filename: "720_h264.mp4",
				numSeeks: 25,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"720_h264.mp4"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideoINPVD",
		}, {
			Name: "hevc",
			Val: seekTest{
				filename: "720_hevc.mp4",
				numSeeks: 25,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"720_hevc.mp4"},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsHEVCVideoDecodingInChrome()),
			ExtraSoftwareDeps: []string{caps.HWDecodeHEVC, "proprietary_codecs"},
			Fixture:           "chromeVideo",
		}, {
			Name: "vp8",
			Val: seekTest{
				filename: "720_vp8.webm",
				numSeeks: 25,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"720_vp8.webm"},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP8},
			Fixture:           "chromeVideo",
		}, {
			Name: "vp9",
			Val: seekTest{
				filename: "720_vp9.webm",
				numSeeks: 25,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"720_vp9.webm"},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9},
			Fixture:           "chromeVideo",
		}, {
			Name: "switch_av1",
			Val: seekTest{
				filename: "smpte_bars_resolution_ladder.av1.webm",
				numSeeks: 25,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"smpte_bars_resolution_ladder.av1.webm"},
			ExtraSoftwareDeps: []string{caps.HWDecodeAV1},
			Fixture:           "chromeVideo",
		}, {
			Name: "switch_h264",
			Val: seekTest{
				filename: "smpte_bars_resolution_ladder.h264.mp4",
				numSeeks: 25,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"smpte_bars_resolution_ladder.h264.mp4"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideo",
		}, {
			Name: "switch_h264_inpvd",
			Val: seekTest{
				filename: "smpte_bars_resolution_ladder.h264.mp4",
				numSeeks: 25,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"smpte_bars_resolution_ladder.h264.mp4"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideoINPVD",
		}, {
			Name: "switch_hevc",
			Val: seekTest{
				filename: "smpte_bars_resolution_ladder.hevc.mp4",
				numSeeks: 25,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"smpte_bars_resolution_ladder.hevc.mp4"},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsHEVCVideoDecodingInChrome()),
			ExtraSoftwareDeps: []string{caps.HWDecodeHEVC, "proprietary_codecs"},
			Fixture:           "chromeVideo",
		}, {
			Name: "switch_vp8",
			Val: seekTest{
				filename: "smpte_bars_resolution_ladder.vp8.webm",
				numSeeks: 25,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"smpte_bars_resolution_ladder.vp8.webm"},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP8},
			Fixture:           "chromeVideo",
		}, {
			Name: "switch_vp9",
			Val: seekTest{
				filename: "smpte_bars_resolution_ladder.vp9.webm",
				numSeeks: 25,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraData:         []string{"smpte_bars_resolution_ladder.vp9.webm"},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9},
			Fixture:           "chromeVideo",
		}, {
			Name: "stress_av1",
			Val: seekTest{
				filename: "720_av1.mp4",
				numSeeks: 1000,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_weekly"},
			ExtraData:         []string{"720_av1.mp4"},
			ExtraSoftwareDeps: []string{caps.HWDecodeAV1},
			Timeout:           20 * time.Minute,
			Fixture:           "chromeVideo",
		}, {
			Name: "stress_vp8",
			Val: seekTest{
				filename: "720_vp8.webm",
				numSeeks: 1000,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_weekly"},
			ExtraData:         []string{"720_vp8.webm"},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP8},
			Timeout:           20 * time.Minute,
			Fixture:           "chromeVideo",
		}, {
			Name: "stress_vp9",
			Val: seekTest{
				filename: "720_vp9.webm",
				numSeeks: 1000,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_weekly"},
			ExtraData:         []string{"720_vp9.webm"},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9},
			Timeout:           20 * time.Minute,
			Fixture:           "chromeVideo",
		}, {
			Name: "stress_h264",
			Val: seekTest{
				filename: "720_h264.mp4",
				numSeeks: 1000,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_weekly"},
			ExtraData:         []string{"720_h264.mp4"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			Timeout:           20 * time.Minute,
			Fixture:           "chromeVideo",
		}, {
			Name: "stress_hevc",
			Val: seekTest{
				filename: "720_hevc.mp4",
				numSeeks: 1000,
			},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_weekly"},
			ExtraData:         []string{"720_hevc.mp4"},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsHEVCVideoDecodingInChrome()),
			ExtraSoftwareDeps: []string{caps.HWDecodeHEVC, "proprietary_codecs"},
			Timeout:           20 * time.Minute,
			Fixture:           "chromeVideo",
		}},
	})
}

// Seek plays a file with Chrome and checks that it can safely be seeked into.
func Seek(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	testOpt := s.Param().(seekTest)

	if err := play.TestSeek(ctx, http.FileServer(s.DataFileSystem()), cr, testOpt.filename, s.OutDir(), testOpt.numSeeks); err != nil {
		s.Fatal("TestSeek failed: ", err)
	}
}
