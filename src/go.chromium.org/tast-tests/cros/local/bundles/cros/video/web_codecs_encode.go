// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/video/webcodecs"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/media/videotype"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         WebCodecsEncode,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that WebCodecs encoding API works, maybe verifying use of a hardware accelerator",
		Contacts: []string{
			"greenjustin@google.com",
			"chromeos-gfx-video@google.com",
			"hiroh@chromium.org", // Test author.
		},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video
		SoftwareDeps: []string{"chrome"},
		Data:         append(append(webcodecs.MP4DemuxerDataFiles(), webcodecs.EncodeDataFiles()...), webcodecs.VideoDataFiles()...),
		Attr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
		Params: []testing.Param{{
			Name:              "h264_sw",
			Val:               webcodecs.TestEncodeArgs{Codec: videotype.H264, Acceleration: webcodecs.PreferSoftware, BitrateMode: "constant", NumOfEncoders: 1},
			ExtraSoftwareDeps: []string{"proprietary_codecs"},
			Fixture:           "chromeVideo",
		}, {
			Name:              "h264_hw",
			Val:               webcodecs.TestEncodeArgs{Codec: videotype.H264, Acceleration: webcodecs.PreferHardware, BitrateMode: "constant", NumOfEncoders: 1},
			ExtraSoftwareDeps: []string{"proprietary_codecs", caps.HWEncodeH264},
			Fixture:           "chromeVideo",
		}, {
			Name:              "h264_hw_3encoders",
			Val:               webcodecs.TestEncodeArgs{Codec: videotype.H264, Acceleration: webcodecs.PreferHardware, BitrateMode: "constant", NumOfEncoders: 3},
			ExtraSoftwareDeps: []string{"proprietary_codecs", caps.HWEncodeH264},
			Fixture:           "chromeVideo",
		}, {
			Name:              "h264_hw_oopve",
			Val:               webcodecs.TestEncodeArgs{Codec: videotype.H264, Acceleration: webcodecs.PreferHardware, BitrateMode: "constant", VerifyOutOfProcessVideoEncodingIsUsed: true, NumOfEncoders: 1},
			ExtraSoftwareDeps: []string{"proprietary_codecs", caps.HWEncodeH264},
			Fixture:           "chromeVideoWithOOPVE",
		}, {
			Name:              "h264_hw_3encoders_oopve",
			Val:               webcodecs.TestEncodeArgs{Codec: videotype.H264, Acceleration: webcodecs.PreferHardware, BitrateMode: "constant", VerifyOutOfProcessVideoEncodingIsUsed: true, NumOfEncoders: 3},
			ExtraSoftwareDeps: []string{"proprietary_codecs", caps.HWEncodeH264},
			Fixture:           "chromeVideoWithOOPVE",
		}, {
			Name:              "h264_sw_l1t2",
			Val:               webcodecs.TestEncodeArgs{Codec: videotype.H264, Acceleration: webcodecs.PreferSoftware, ScalabilityMode: "L1T2", BitrateMode: "constant", NumOfEncoders: 1},
			ExtraSoftwareDeps: []string{"proprietary_codecs"},
			Fixture:           "chromeVideo",
		}, {
			Name:              "h264_hw_l1t2",
			Val:               webcodecs.TestEncodeArgs{Codec: videotype.H264, Acceleration: webcodecs.PreferHardware, ScalabilityMode: "L1T2", BitrateMode: "constant", NumOfEncoders: 1},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsSVCEncoding("h264main", "l1t2")),
			ExtraSoftwareDeps: []string{"proprietary_codecs", caps.HWEncodeH264},
			Fixture:           "chromeVideoHardwareTemporalEncoding",
		}, {
			Name:              "h264_sw_l1t3",
			Val:               webcodecs.TestEncodeArgs{Codec: videotype.H264, Acceleration: webcodecs.PreferSoftware, ScalabilityMode: "L1T3", BitrateMode: "constant", NumOfEncoders: 1},
			ExtraSoftwareDeps: []string{"proprietary_codecs"},
			Fixture:           "chromeVideo",
		}, {
			Name:              "h264_hw_l1t3",
			Val:               webcodecs.TestEncodeArgs{Codec: videotype.H264, Acceleration: webcodecs.PreferHardware, ScalabilityMode: "L1T3", BitrateMode: "constant", NumOfEncoders: 1},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsSVCEncoding("h264main", "l1t3")),
			ExtraSoftwareDeps: []string{"proprietary_codecs", caps.HWEncodeH264},
			Fixture:           "chromeVideo",
		}, {
			Name:              "h264_sw_vbr",
			Val:               webcodecs.TestEncodeArgs{Codec: videotype.H264, Acceleration: webcodecs.PreferSoftware, BitrateMode: "variable", NumOfEncoders: 1},
			ExtraSoftwareDeps: []string{"proprietary_codecs"},
			Fixture:           "chromeVideo",
		}, {
			Name:              "h264_hw_vbr",
			Val:               webcodecs.TestEncodeArgs{Codec: videotype.H264, Acceleration: webcodecs.PreferHardware, BitrateMode: "variable", NumOfEncoders: 1},
			ExtraSoftwareDeps: []string{"proprietary_codecs", caps.HWEncodeH264VBR},
			Fixture:           "chromeVideo",
		}, {
			Name:    "vp8_sw",
			Val:     webcodecs.TestEncodeArgs{Codec: videotype.VP8, Acceleration: webcodecs.PreferSoftware, BitrateMode: "constant", NumOfEncoders: 1},
			Fixture: "chromeVideo",
		}, {
			Name:              "vp8_hw",
			Val:               webcodecs.TestEncodeArgs{Codec: videotype.VP8, Acceleration: webcodecs.PreferHardware, BitrateMode: "constant", NumOfEncoders: 1},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8},
			Fixture:           "chromeVideo",
		}, {
			Name:              "vp8_hw_3encoders",
			Val:               webcodecs.TestEncodeArgs{Codec: videotype.VP8, Acceleration: webcodecs.PreferHardware, BitrateMode: "constant", NumOfEncoders: 3},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8},
			Fixture:           "chromeVideo",
		}, {
			Name:              "vp8_hw_oopve",
			Val:               webcodecs.TestEncodeArgs{Codec: videotype.VP8, Acceleration: webcodecs.PreferHardware, BitrateMode: "constant", VerifyOutOfProcessVideoEncodingIsUsed: true, NumOfEncoders: 1},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8},
			Fixture:           "chromeVideoWithOOPVE",
		}, {
			Name:              "vp8_hw_3encoders_oopve",
			Val:               webcodecs.TestEncodeArgs{Codec: videotype.VP8, Acceleration: webcodecs.PreferHardware, BitrateMode: "constant", VerifyOutOfProcessVideoEncodingIsUsed: true, NumOfEncoders: 3},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8},
			Fixture:           "chromeVideoWithOOPVE",
		}, {
			Name:    "vp8_sw_l1t3",
			Val:     webcodecs.TestEncodeArgs{Codec: videotype.VP8, Acceleration: webcodecs.PreferSoftware, ScalabilityMode: "L1T3", BitrateMode: "constant", NumOfEncoders: 1},
			Fixture: "chromeVideo",
		}, {
			Name:              "vp8_hw_l1t3",
			Val:               webcodecs.TestEncodeArgs{Codec: videotype.VP8, Acceleration: webcodecs.PreferHardware, ScalabilityMode: "L1T3", BitrateMode: "constant", NumOfEncoders: 1},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsSVCEncoding("vp8", "l1t3")),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8},
			Fixture:           "chromeVideoHardwareTemporalEncoding",
		}, {
			Name:    "vp8_sw_vbr",
			Val:     webcodecs.TestEncodeArgs{Codec: videotype.VP8, Acceleration: webcodecs.PreferSoftware, BitrateMode: "variable", NumOfEncoders: 1},
			Fixture: "chromeVideo",
		}, {
			Name:    "vp9_sw",
			Val:     webcodecs.TestEncodeArgs{Codec: videotype.VP9, Acceleration: webcodecs.PreferSoftware, BitrateMode: "constant", NumOfEncoders: 1},
			Fixture: "chromeVideo",
		}, {
			Name:              "vp9_hw",
			Val:               webcodecs.TestEncodeArgs{Codec: videotype.VP9, Acceleration: webcodecs.PreferHardware, BitrateMode: "constant", NumOfEncoders: 1},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			Fixture:           "chromeVideo",
		}, {
			Name:              "vp9_hw_3encoders",
			Val:               webcodecs.TestEncodeArgs{Codec: videotype.VP9, Acceleration: webcodecs.PreferHardware, BitrateMode: "constant", NumOfEncoders: 3},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			Fixture:           "chromeVideo",
		}, {
			Name:              "vp9_hw_oopve",
			Val:               webcodecs.TestEncodeArgs{Codec: videotype.VP9, Acceleration: webcodecs.PreferHardware, BitrateMode: "constant", VerifyOutOfProcessVideoEncodingIsUsed: true, NumOfEncoders: 1},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			Fixture:           "chromeVideoWithOOPVE",
		}, {
			Name:              "vp9_hw_3encoders_oopve",
			Val:               webcodecs.TestEncodeArgs{Codec: videotype.VP9, Acceleration: webcodecs.PreferHardware, BitrateMode: "constant", VerifyOutOfProcessVideoEncodingIsUsed: true, NumOfEncoders: 3},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			Fixture:           "chromeVideoWithOOPVE",
		}, {
			Name:    "vp9_sw_l1t2",
			Val:     webcodecs.TestEncodeArgs{Codec: videotype.VP9, Acceleration: webcodecs.PreferSoftware, ScalabilityMode: "L1T2", BitrateMode: "constant", NumOfEncoders: 1},
			Fixture: "chromeVideo",
		}, {
			Name:              "vp9_hw_l1t2",
			Val:               webcodecs.TestEncodeArgs{Codec: videotype.VP9, Acceleration: webcodecs.PreferHardware, ScalabilityMode: "L1T2", BitrateMode: "constant", NumOfEncoders: 1},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsSVCEncoding("vp9", "l1t2")),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			Fixture:           "chromeVideoHardwareTemporalEncoding",
		}, {
			Name:    "vp9_sw_l1t3",
			Val:     webcodecs.TestEncodeArgs{Codec: videotype.VP9, Acceleration: webcodecs.PreferSoftware, ScalabilityMode: "L1T3", BitrateMode: "constant", NumOfEncoders: 1},
			Fixture: "chromeVideo",
		}, {
			Name:              "vp9_hw_l1t3",
			Val:               webcodecs.TestEncodeArgs{Codec: videotype.VP9, Acceleration: webcodecs.PreferHardware, ScalabilityMode: "L1T3", BitrateMode: "constant", NumOfEncoders: 1},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsSVCEncoding("vp9", "l1t3")),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			Fixture:           "chromeVideoHardwareTemporalEncoding",
		}, {
			Name:    "vp9_sw_vbr",
			Val:     webcodecs.TestEncodeArgs{Codec: videotype.VP9, Acceleration: webcodecs.PreferSoftware, BitrateMode: "variable", NumOfEncoders: 1},
			Fixture: "chromeVideo",
		}, {
			Name:    "av1_sw",
			Val:     webcodecs.TestEncodeArgs{Codec: videotype.AV1, Acceleration: webcodecs.PreferSoftware, BitrateMode: "constant", NumOfEncoders: 1},
			Fixture: "chromeVideo",
		}, {
			Name:              "av1_hw",
			Val:               webcodecs.TestEncodeArgs{Codec: videotype.AV1, Acceleration: webcodecs.PreferHardware, BitrateMode: "constant", NumOfEncoders: 1},
			Fixture:           "chromeVideo",
			ExtraSoftwareDeps: []string{caps.HWEncodeAV1},
		}, {
			Name:    "av1_sw_l1t2",
			Val:     webcodecs.TestEncodeArgs{Codec: videotype.AV1, Acceleration: webcodecs.PreferSoftware, ScalabilityMode: "L1T2", BitrateMode: "constant", NumOfEncoders: 1},
			Fixture: "chromeVideo",
		}, {
			Name:    "av1_sw_l1t3",
			Val:     webcodecs.TestEncodeArgs{Codec: videotype.AV1, Acceleration: webcodecs.PreferSoftware, ScalabilityMode: "L1T3", BitrateMode: "constant", NumOfEncoders: 1},
			Fixture: "chromeVideo",
		}, {
			Name:    "av1_sw_vbr",
			Val:     webcodecs.TestEncodeArgs{Codec: videotype.AV1, Acceleration: webcodecs.PreferSoftware, BitrateMode: "variable", NumOfEncoders: 1},
			Fixture: "chromeVideo",
		}},
	})
}

func WebCodecsEncode(ctx context.Context, s *testing.State) {
	args := s.Param().(webcodecs.TestEncodeArgs)
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	if err := webcodecs.RunEncodeTest(ctx, cr,
		s.DataFileSystem(), args, s.DataPath(webcodecs.Crowd720p), s.OutDir()); err != nil {
		s.Error("Test failed: ", err)
	}
}
