// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/video/webcodecs"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/media/pre"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         WebCodecsDecode,
		LacrosStatus: testing.LacrosVariantUnknown,
		Desc:         "Verifies that WebCodecs API works, maybe verifying use of a hardware accelerator",
		Contacts: []string{
			"greenjustin@google.com",
			"chromeos-gfx-video@google.com",
			"hiroh@chromium.org", // Test author.
		},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video
		SoftwareDeps: []string{"chrome"},
		Data:         append(webcodecs.DecodeDataFiles(), webcodecs.MP4DemuxerDataFiles()...),
		Attr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
		Params: []testing.Param{{
			Name:      "av1_sw",
			Val:       webcodecs.TestDecodeArgs{VideoFile: "bear-320x240.av1.mp4", Acceleration: webcodecs.PreferSoftware, BrowserType: browser.TypeAsh},
			ExtraData: []string{"bear-320x240.av1.mp4", "bear-320x240.av1.mp4.json"},
			Fixture:   "chromeVideo",
		}, {
			Name:              "av1_hw",
			Val:               webcodecs.TestDecodeArgs{VideoFile: "bear-320x240.av1.mp4", Acceleration: webcodecs.PreferHardware, BrowserType: browser.TypeAsh},
			ExtraSoftwareDeps: []string{caps.HWDecodeAV1},
			ExtraData:         []string{"bear-320x240.av1.mp4", "bear-320x240.av1.mp4.json"},
			Fixture:           "chromeVideo",
		}, {
			Name:              "h264_sw",
			Val:               webcodecs.TestDecodeArgs{VideoFile: "bear-320x240.h264.mp4", Acceleration: webcodecs.PreferSoftware, BrowserType: browser.TypeAsh},
			ExtraSoftwareDeps: []string{"proprietary_codecs"},
			ExtraData:         []string{"bear-320x240.h264.mp4", "bear-320x240.h264.mp4.json"},
			Fixture:           "chromeVideo",
		}, {
			Name:              "h264_hw",
			Val:               webcodecs.TestDecodeArgs{VideoFile: "bear-320x240.h264.mp4", Acceleration: webcodecs.PreferHardware, BrowserType: browser.TypeAsh},
			ExtraSoftwareDeps: []string{"proprietary_codecs", caps.HWDecodeH264},
			ExtraData:         []string{"bear-320x240.h264.mp4", "bear-320x240.h264.mp4.json"},
			Fixture:           "chromeVideo",
		}, {
			Name:              "h264_hw_inpvd",
			Val:               webcodecs.TestDecodeArgs{VideoFile: "bear-320x240.h264.mp4", Acceleration: webcodecs.PreferHardware, BrowserType: browser.TypeAsh},
			ExtraSoftwareDeps: []string{"proprietary_codecs", caps.HWDecodeH264},
			ExtraData:         []string{"bear-320x240.h264.mp4", "bear-320x240.h264.mp4.json"},
			Fixture:           "chromeVideoINPVD",
		}, {
			Name:      "vp8_sw",
			Val:       webcodecs.TestDecodeArgs{VideoFile: "bear-320x240.vp8.mp4", Acceleration: webcodecs.PreferSoftware, BrowserType: browser.TypeAsh},
			ExtraData: []string{"bear-320x240.vp8.mp4", "bear-320x240.vp8.mp4.json"},
			Fixture:   "chromeVideo",
		}, {
			Name:              "vp8_hw",
			Val:               webcodecs.TestDecodeArgs{VideoFile: "bear-320x240.vp8.mp4", Acceleration: webcodecs.PreferHardware, BrowserType: browser.TypeAsh},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP8},
			ExtraData:         []string{"bear-320x240.vp8.mp4", "bear-320x240.vp8.mp4.json"},
			Fixture:           "chromeVideo",
		}, {
			Name:      "vp9_sw",
			Val:       webcodecs.TestDecodeArgs{VideoFile: "bear-320x240.vp9.mp4", Acceleration: webcodecs.PreferSoftware, BrowserType: browser.TypeAsh},
			ExtraData: []string{"bear-320x240.vp9.mp4", "bear-320x240.vp9.mp4.json"},
			Fixture:   "chromeVideo",
		}, {
			Name:              "vp9_hw",
			Val:               webcodecs.TestDecodeArgs{VideoFile: "bear-320x240.vp9.mp4", Acceleration: webcodecs.PreferHardware, BrowserType: browser.TypeAsh},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9},
			ExtraData:         []string{"bear-320x240.vp9.mp4", "bear-320x240.vp9.mp4.json"},
			Fixture:           "chromeVideo",
		}, {
			Name:              "av1_hw_lacros",
			Val:               webcodecs.TestDecodeArgs{VideoFile: "bear-320x240.av1.mp4", Acceleration: webcodecs.PreferHardware, BrowserType: browser.TypeLacros},
			ExtraSoftwareDeps: []string{caps.HWDecodeAV1, "lacros"},
			ExtraData:         []string{"bear-320x240.av1.mp4", "bear-320x240.av1.mp4.json"},
			Fixture:           pre.ChromeVideoLacrosFixture(pre.FakeMediaStreamUI),
		}, {
			Name:              "h264_hw_lacros",
			Val:               webcodecs.TestDecodeArgs{VideoFile: "bear-320x240.h264.mp4", Acceleration: webcodecs.PreferHardware, BrowserType: browser.TypeLacros},
			ExtraSoftwareDeps: []string{"proprietary_codecs", caps.HWDecodeH264, "lacros"},
			ExtraData:         []string{"bear-320x240.h264.mp4", "bear-320x240.h264.mp4.json"},
			Fixture:           pre.ChromeVideoLacrosFixture(pre.FakeMediaStreamUI),
		}, {
			Name:              "vp8_hw_lacros",
			Val:               webcodecs.TestDecodeArgs{VideoFile: "bear-320x240.vp8.mp4", Acceleration: webcodecs.PreferHardware, BrowserType: browser.TypeLacros},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP8, "lacros"},
			ExtraData:         []string{"bear-320x240.vp8.mp4", "bear-320x240.vp8.mp4.json"},
			Fixture:           pre.ChromeVideoLacrosFixture(pre.FakeMediaStreamUI),
		}, {
			Name:              "vp9_hw_lacros",
			Val:               webcodecs.TestDecodeArgs{VideoFile: "bear-320x240.vp9.mp4", Acceleration: webcodecs.PreferHardware, BrowserType: browser.TypeLacros},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9, "lacros"},
			ExtraData:         []string{"bear-320x240.vp9.mp4", "bear-320x240.vp9.mp4.json"},
			Fixture:           pre.ChromeVideoLacrosFixture(pre.FakeMediaStreamUI),
		}, {
			Name:              "h264_hw_lacros_gtfo",
			Val:               webcodecs.TestDecodeArgs{VideoFile: "bear-320x240.h264.mp4", Acceleration: webcodecs.PreferHardware, BrowserType: browser.TypeLacros},
			ExtraSoftwareDeps: []string{"proprietary_codecs", caps.HWDecodeH264, "lacros"},
			ExtraData:         []string{"bear-320x240.h264.mp4", "bear-320x240.h264.mp4.json"},
			Fixture:           pre.ChromeVideoLacrosFixture(pre.FakeMediaStreamUI, pre.GTFO),
		}},
	})
}

func WebCodecsDecode(ctx context.Context, s *testing.State) {
	args := s.Param().(webcodecs.TestDecodeArgs)

	_, l, cs, err := lacros.Setup(ctx, s.FixtValue(), args.BrowserType)
	if err != nil {
		s.Fatal("Failed to initialize test: ", err)
	}
	defer lacros.CloseLacros(ctx, l)

	if err := webcodecs.RunDecodeTest(ctx, cs,
		s.DataFileSystem(), args, s.DataPath(args.VideoFile+".json"), s.OutDir()); err != nil {
		s.Error("Test failed: ", err)
	}
}
