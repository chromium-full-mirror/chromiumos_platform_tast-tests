// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/media/decoding"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// cqAllowlist is the list of stable device models we want to enable CQ for.
// Note: The CQ runs a test pre-commit dozens/hundreds of times per post-commit release build.
// Adding tests to the CQ is therefore extremely expensive. As tests in the CQ may prevent Chrome
// from upreving, only devices that are present in Chromium CQ are added. Consider carefully which
// tests/devices to add to the CQ.
var cqAllowlist = []string{
	"eve",
	"kevin",
}

type chromeStackDecoderLegacyTestParam struct {
	dataPath string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChromeStackDecoderLegacy,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies hardware decode acceleration by running the video_decode_accelerator_tests binary with the legacy implementation",
		Contacts: []string{
			"chromeos-gfx-video@google.com",
			"mcasas@chromium.org",
		},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video
		// Don't run these tests on "vaapi" since the legacy decoder is not used on
		// those platforms and it's on its way to deprecation, see b/275623115.
		SoftwareDeps: []string{"chrome", "video_decoder_legacy_supported", "v4l2_codec"},
		Timeout:      4 * time.Minute,
		Fixture:      "graphicsNoChrome",
		Params: []testing.Param{{
			Name:              "h264",
			Val:               chromeStackDecoderLegacyTestParam{dataPath: "test-25fps.h264"},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild", "graphics_video_decodeaccel"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			ExtraData:         []string{"test-25fps.h264", "test-25fps.h264.json"},
		}, {
			// Run H264 video decode tests on CQ, limited to devices on the CQ allow list.
			Name:              "h264_cq",
			Val:               chromeStackDecoderLegacyTestParam{dataPath: "test-25fps.h264"},
			ExtraHardwareDeps: hwdep.D(hwdep.Model(cqAllowlist...)),
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild", "graphics_video_decodeaccel"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			ExtraData:         []string{"test-25fps.h264", "test-25fps.h264.json"},
		}, {
			Name:              "vp8",
			Val:               chromeStackDecoderLegacyTestParam{dataPath: "test-25fps.vp8"},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild", "graphics_video_decodeaccel"},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP8, "video_decoder_legacy_supported"},
			ExtraData:         []string{"test-25fps.vp8", "test-25fps.vp8.json"},
		}, {
			Name:              "vp9",
			Val:               chromeStackDecoderLegacyTestParam{dataPath: "test-25fps.vp9"},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild", "graphics_video_decodeaccel"},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9, "video_decoder_legacy_supported"},
			ExtraData:         []string{"test-25fps.vp9", "test-25fps.vp9.json"},
		}, {
			Name:              "h264_resolution_switch",
			Val:               chromeStackDecoderLegacyTestParam{dataPath: "switch_1080p_720p_240frames.h264"},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild", "graphics_video_decodeaccel"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			ExtraData:         []string{"switch_1080p_720p_240frames.h264", "switch_1080p_720p_240frames.h264.json"},
		}, {
			Name:              "vp8_resolution_switch",
			Val:               chromeStackDecoderLegacyTestParam{dataPath: "resolution_change_500frames.vp8.ivf"},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild", "graphics_video_decodeaccel"},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP8, "video_decoder_legacy_supported"},
			ExtraData:         []string{"resolution_change_500frames.vp8.ivf", "resolution_change_500frames.vp8.ivf.json"},
		}, {
			Name:              "vp9_resolution_switch",
			Val:               chromeStackDecoderLegacyTestParam{dataPath: "resolution_change_500frames.vp9.ivf"},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild", "graphics_video_decodeaccel"},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9, "video_decoder_legacy_supported"},
			ExtraData:         []string{"resolution_change_500frames.vp9.ivf", "resolution_change_500frames.vp9.ivf.json"},
		}, {
			Name: "vp8_odd_dimensions",
			Val:  chromeStackDecoderLegacyTestParam{dataPath: "test-25fps-321x241.vp8"},
			// TODO(b/138915749): Enable once decoding odd dimension videos is fixed.
			ExtraSoftwareDeps: []string{caps.HWDecodeVP8, "video_decoder_legacy_supported"},
			ExtraData:         []string{"test-25fps-321x241.vp8", "test-25fps-321x241.vp8.json"},
		}, {
			Name: "vp9_odd_dimensions",
			Val:  chromeStackDecoderLegacyTestParam{dataPath: "test-25fps-321x241.vp9"},
			// TODO(b/138915749): Enable once decoding odd dimension videos is fixed.
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9, "video_decoder_legacy_supported"},
			ExtraData:         []string{"test-25fps-321x241.vp9", "test-25fps-321x241.vp9.json"},
		}, {
			// This test uses a video that makes use of the VP9 show-existing-frame feature and is used in Android CTS:
			// https://android.googlesource.com/platform/cts/+/HEAD/tests/tests/media/res/raw/vp90_2_17_show_existing_frame.vp9
			Name:              "vp9_show_existing_frame",
			Val:               chromeStackDecoderLegacyTestParam{dataPath: "vda_smoke-vp90_2_17_show_existing_frame.vp9"},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild", "graphics_video_decodeaccel"},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9, "video_decoder_legacy_supported"},
			ExtraData:         []string{"vda_smoke-vp90_2_17_show_existing_frame.vp9", "vda_smoke-vp90_2_17_show_existing_frame.vp9.json"},
		}, {
			// H264 stream in which a profile changes from Baseline to Main.
			Name:              "h264_profile_change",
			Val:               chromeStackDecoderLegacyTestParam{dataPath: "test-25fps_basemain.h264"},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild", "graphics_video_decodeaccel"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			ExtraData:         []string{"test-25fps_basemain.h264", "test-25fps_basemain.h264.json"},
		}},
	})
}

func ChromeStackDecoderLegacy(ctx context.Context, s *testing.State) {
	param := s.Param().(chromeStackDecoderLegacyTestParam)

	if err := decoding.RunAccelVideoTest(ctx, s.OutDir(), s.DataPath(param.dataPath), decoding.TestParams{DecoderType: decoding.VDA, DisableGlobalVaapiLock: false}, []string{}); err != nil {
		s.Fatal("test failed: ", err)
	}
}
