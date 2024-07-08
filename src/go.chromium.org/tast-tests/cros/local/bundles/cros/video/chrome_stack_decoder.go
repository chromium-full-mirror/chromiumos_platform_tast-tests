// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/graphics/expectations"
	"go.chromium.org/tast-tests/cros/local/media/decoding"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type chromeStackDecoderTestParam struct {
	dataPath string
	// List of Chrome Features to enable, if any.
	enabledFeatures []string
	clientInterface decoding.MediaDecoderInterface
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChromeStackDecoder,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies hardware decode acceleration of media::VideoDecoders by running the video_decode_accelerator_tests binary (see go/vd-migration)",
		Contacts: []string{
			"chromeos-gfx-video@google.com",
			"mcasas@chromium.org",
		},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video
		SoftwareDeps: []string{"chrome"},
		Fixture:      "graphicsNoChrome",
		Attr:         []string{"group:graphics", "graphics_video", "graphics_perbuild", "graphics_video_decodeaccel"},
		Params: []testing.Param{{
			Name:              "av1",
			Val:               chromeStackDecoderTestParam{dataPath: "test-25fps.av1.ivf"},
			ExtraSoftwareDeps: []string{caps.HWDecodeAV1},
			ExtraData:         []string{"test-25fps.av1.ivf", "test-25fps.av1.ivf.json"},
			Timeout:           4 * time.Minute,
		}, {
			Name:              "av1_10bit",
			Val:               chromeStackDecoderTestParam{dataPath: "test-25fps-10bit.av1.ivf"},
			ExtraSoftwareDeps: []string{caps.HWDecodeAV1_10BPP},
			ExtraData:         []string{"test-25fps-10bit.av1.ivf", "test-25fps-10bit.av1.ivf.json"},
			Timeout:           4 * time.Minute,
		}, {
			Name:              "h264",
			Val:               chromeStackDecoderTestParam{dataPath: "test-25fps.h264"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			ExtraData:         []string{"test-25fps.h264", "test-25fps.h264.json"},
			Timeout:           4 * time.Minute,
		}, {
			Name:              "hevc",
			Val:               chromeStackDecoderTestParam{dataPath: "test-25fps.hevc"},
			ExtraSoftwareDeps: []string{caps.HWDecodeHEVC, "proprietary_codecs"},
			ExtraData:         []string{"test-25fps.hevc", "test-25fps.hevc.json"},
			Timeout:           4 * time.Minute,
		}, {
			Name:              "hevc_10bit",
			Val:               chromeStackDecoderTestParam{dataPath: "test-25fps.hevc10"},
			ExtraSoftwareDeps: []string{caps.HWDecodeHEVC10BPP, "proprietary_codecs"},
			ExtraData:         []string{"test-25fps.hevc10", "test-25fps.hevc10.json"},
			Timeout:           4 * time.Minute,
		}, {
			Name:              "vp8",
			Val:               chromeStackDecoderTestParam{dataPath: "test-25fps.vp8"},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP8},
			ExtraData:         []string{"test-25fps.vp8", "test-25fps.vp8.json"},
			Timeout:           4 * time.Minute,
		}, {
			Name:              "vp9",
			Val:               chromeStackDecoderTestParam{dataPath: "test-25fps.vp9"},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9},
			ExtraData:         []string{"test-25fps.vp9", "test-25fps.vp9.json"},
			Timeout:           4 * time.Minute,
		}, {
			Name:              "vp9_2",
			Val:               chromeStackDecoderTestParam{dataPath: "test-25fps.vp9_2"},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9_2},
			ExtraData:         []string{"test-25fps.vp9_2", "test-25fps.vp9_2.json"},
			Timeout:           4 * time.Minute,
		}, {
			Name:              "av1_resolution_switch",
			Val:               chromeStackDecoderTestParam{dataPath: "resolution_change.av1.ivf"},
			ExtraSoftwareDeps: []string{caps.HWDecodeAV1},
			ExtraData:         []string{"resolution_change.av1.ivf", "resolution_change.av1.ivf.json"},
			Timeout:           10 * time.Minute,
		}, {
			Name:              "h264_resolution_switch",
			Val:               chromeStackDecoderTestParam{dataPath: "switch_1080p_720p_240frames.h264"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			ExtraData:         []string{"switch_1080p_720p_240frames.h264", "switch_1080p_720p_240frames.h264.json"},
			Timeout:           10 * time.Minute,
		}, {
			Name:              "hevc_resolution_switch",
			Val:               chromeStackDecoderTestParam{dataPath: "switch_1080p_720p_240frames.hevc"},
			ExtraSoftwareDeps: []string{caps.HWDecodeHEVC, "proprietary_codecs"},
			ExtraData:         []string{"switch_1080p_720p_240frames.hevc", "switch_1080p_720p_240frames.hevc.json"},
			Timeout:           10 * time.Minute,
		}, {
			Name:              "vp8_resolution_switch",
			Val:               chromeStackDecoderTestParam{dataPath: "resolution_change_500frames.vp8.ivf"},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP8},
			ExtraData:         []string{"resolution_change_500frames.vp8.ivf", "resolution_change_500frames.vp8.ivf.json"},
			Timeout:           10 * time.Minute,
		}, {
			Name:              "vp9_resolution_switch",
			Val:               chromeStackDecoderTestParam{dataPath: "resolution_change_500frames.vp9.ivf"},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9},
			ExtraData:         []string{"resolution_change_500frames.vp9.ivf", "resolution_change_500frames.vp9.ivf.json"},
			Timeout:           10 * time.Minute,
		}, {
			// This test uses a video that makes use of the VP9 show-existing-frame feature and is used in Android CTS:
			// https://android.googlesource.com/platform/cts/+/HEAD/tests/tests/media/res/raw/vp90_2_17_show_existing_frame.vp9
			Name:              "vp9_show_existing_frame",
			Val:               chromeStackDecoderTestParam{dataPath: "vda_smoke-vp90_2_17_show_existing_frame.vp9"},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9},
			ExtraData:         []string{"vda_smoke-vp90_2_17_show_existing_frame.vp9", "vda_smoke-vp90_2_17_show_existing_frame.vp9.json"},
			Timeout:           4 * time.Minute,
		}, {
			// H264 stream in which a profile changes from Baseline to Main.
			Name:              "h264_profile_change",
			Val:               chromeStackDecoderTestParam{dataPath: "test-25fps_basemain.h264"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			ExtraData:         []string{"test-25fps_basemain.h264", "test-25fps_basemain.h264.json"},
			Timeout:           4 * time.Minute,
		}, {
			// Decode VP9 spatial-SVC stream. Precisely the structure in the stream is called k-SVC, where spatial-layers are at key-frame only.
			// The structure is used in Hangouts Meet. go/vp9-svc-hangouts for detail.
			Name:              "vp9_keyframe_spatial_layers",
			Val:               chromeStackDecoderTestParam{dataPath: "keyframe_spatial_layers_180p_360p.vp9.ivf"},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsVP9KSVCHWDecoding(), hwdep.SkipGPUFamily("rogue")),
			ExtraData:         []string{"keyframe_spatial_layers_180p_360p.vp9.ivf", "keyframe_spatial_layers_180p_360p.vp9.ivf.json"},
			Timeout:           4 * time.Minute,
		}, {
			Name:              "av1_odd_dimension",
			Val:               chromeStackDecoderTestParam{dataPath: "test-25fps-321x241.av1.ivf"},
			ExtraSoftwareDeps: []string{caps.HWDecodeAV1},
			ExtraData:         []string{"test-25fps-321x241.av1.ivf", "test-25fps-321x241.av1.ivf.json"},
			Timeout:           4 * time.Minute,
		}, {
			Name:              "vp8_odd_dimension",
			Val:               chromeStackDecoderTestParam{dataPath: "test-25fps-321x241.vp8"},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP8},
			ExtraData:         []string{"test-25fps-321x241.vp8", "test-25fps-321x241.vp8.json"},
			Timeout:           4 * time.Minute,
		}, {
			Name:              "vp9_odd_dimension",
			Val:               chromeStackDecoderTestParam{dataPath: "test-25fps-321x241.vp9"},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9},
			ExtraData:         []string{"test-25fps-321x241.vp9", "test-25fps-321x241.vp9.json"},
			Timeout:           4 * time.Minute,
		}, {
			Name:              "h264_vdvda",
			Val:               chromeStackDecoderTestParam{dataPath: "test-25fps.h264", clientInterface: decoding.AdapterFromLegacyToCurrent},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsV4L2FlatVideoDecoding()),
			ExtraData:         []string{"test-25fps.h264", "test-25fps.h264.json"},
			Timeout:           4 * time.Minute,
		}, {
			Name:              "vp8_vdvda",
			Val:               chromeStackDecoderTestParam{dataPath: "test-25fps.vp8", clientInterface: decoding.AdapterFromLegacyToCurrent},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP8},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsV4L2FlatVideoDecoding()),
			ExtraData:         []string{"test-25fps.vp8", "test-25fps.vp8.json"},
			Timeout:           4 * time.Minute,
		}, {
			Name:              "vp9_vdvda",
			Val:               chromeStackDecoderTestParam{dataPath: "test-25fps.vp9", clientInterface: decoding.AdapterFromLegacyToCurrent},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsV4L2FlatVideoDecoding()),
			ExtraData:         []string{"test-25fps.vp9", "test-25fps.vp9.json"},
			Timeout:           4 * time.Minute,
		},
		},
	})
}

func ChromeStackDecoder(ctx context.Context, s *testing.State) {
	expectation, err := expectations.GetTestExpectation(ctx, s.TestName())
	if err != nil {
		s.Fatal("Failed to load test expectation: ", err)
	}
	params := s.Param().(chromeStackDecoderTestParam)

	if err := decoding.RunAccelVideoTest(ctx, s.OutDir(), s.DataPath(params.dataPath), decoding.TestParams{MediaDecoderInterface: params.clientInterface}, params.enabledFeatures); err != nil {
		if expErr := expectation.ReportError("test failed: ", err); expErr != nil {
			s.Fatal("Unexpected error: ", expErr)
		}
	}
}
