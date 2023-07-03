// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/video/encode"
	"go.chromium.org/tast-tests/cros/local/media/videotype"
	"go.chromium.org/tast/core/testing"
)

const (
	desktop135P  = "encode/desktop2-240x135_850frames.vp9.webm"
	desktop180P  = "encode/desktop2-320x180_850frames.vp9.webm"
	desktop270P  = "encode/desktop2-480x270_850frames.vp9.webm"
	desktop360P  = "encode/desktop2-640x360_850frames.vp9.webm"
	desktop540P  = "encode/desktop2-960x540_850frames.vp9.webm"
	desktop720P  = "encode/desktop2-1280x720_850frames.vp9.webm"
	desktop1080P = "encode/desktop2-1920x1080_850frames.vp9.webm"
	desktop2160P = "encode/desktop2-3840x2160_430frames.vp9.webm"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         EncodeAccel,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies hardware encode acceleration by running the video_encode_accelerator_tests binary",
		Contacts: []string{
			"chromeos-gfx-video@google.com",
			"hiroh@chromium.org",
		},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video
		SoftwareDeps: []string{"chrome"},
		Timeout:      10 * time.Minute,
		Attr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
		Fixture:      "graphicsNoChrome",
		Params: []testing.Param{{
			Name:              "h264_180p",
			Val:               encode.MakeTestOptions(desktop180P, videotype.H264BaselineProf),
			ExtraSoftwareDeps: []string{caps.HWEncodeH264},
			ExtraData:         encode.TestData(desktop180P),
		}, {
			Name:              "h264_270p",
			Val:               encode.MakeTestOptions(desktop270P, videotype.H264BaselineProf),
			ExtraSoftwareDeps: []string{caps.HWEncodeH264},
			ExtraData:         encode.TestData(desktop270P),
		}, {
			Name:              "h264_360p",
			Val:               encode.MakeTestOptions(desktop360P, videotype.H264BaselineProf),
			ExtraSoftwareDeps: []string{caps.HWEncodeH264},
			ExtraData:         encode.TestData(desktop360P),
		}, {
			Name:              "h264_720p",
			Val:               encode.MakeTestOptions(desktop720P, videotype.H264BaselineProf),
			ExtraSoftwareDeps: []string{caps.HWEncodeH264},
			ExtraData:         encode.TestData(desktop720P),
		}, {
			Name:              "h264_720p_l1t2",
			Val:               encode.MakeTestOptionsWithSVCMode(desktop720P, videotype.H264BaselineProf, "L1T2"),
			ExtraSoftwareDeps: []string{caps.HWEncodeH264, "vaapi"},
			ExtraData:         encode.TestData(desktop720P),
		}, {
			Name:              "h264_720p_l1t3",
			Val:               encode.MakeTestOptionsWithSVCMode(desktop720P, videotype.H264BaselineProf, "L1T3"),
			ExtraSoftwareDeps: []string{caps.HWEncodeH264, "vaapi"},
			ExtraData:         encode.TestData(desktop720P),
		}, {
			Name:              "h264_720p_vbr",
			Val:               encode.MakeVBRTestOptions(desktop720P, videotype.H264BaselineProf),
			ExtraSoftwareDeps: []string{caps.HWEncodeH264VBR},
			ExtraData:         encode.TestData(desktop720P),
		}, {
			Name:              "h264_720p_l1t2_vbr",
			Val:               encode.MakeVBRTestOptionsWithSVCMode(desktop720P, videotype.H264BaselineProf, "L1T2"),
			ExtraSoftwareDeps: []string{caps.HWEncodeH264VBR, "vaapi"},
			ExtraData:         encode.TestData(desktop720P),
		}, {
			Name:              "h264_720p_l1t3_vbr",
			Val:               encode.MakeVBRTestOptionsWithSVCMode(desktop720P, videotype.H264BaselineProf, "L1T3"),
			ExtraSoftwareDeps: []string{caps.HWEncodeH264VBR, "vaapi"},
			ExtraData:         encode.TestData(desktop720P),
		}, {
			Name:              "h264_1080p",
			Val:               encode.MakeTestOptions(desktop1080P, videotype.H264BaselineProf),
			ExtraSoftwareDeps: []string{caps.HWEncodeH264},
			ExtraData:         encode.TestData(desktop1080P),
		}, {
			Name:              "h264_1080p_global_vaapi_lock_disabled",
			Val:               encode.MakeTestOptionsWithNoGlobalVaapiLock(desktop1080P, videotype.H264BaselineProf),
			ExtraSoftwareDeps: []string{caps.HWEncodeH264, "thread_safe_libva_backend"},
			ExtraData:         encode.TestData(desktop1080P),
		}, {
			Name:              "h264_2160p",
			Val:               encode.MakeTestOptions(desktop2160P, videotype.H264BaselineProf),
			ExtraSoftwareDeps: []string{caps.HWEncodeH264_4K},
			ExtraData:         encode.TestData(desktop2160P),
		}, {
			Name:              "vp8_135p",
			Val:               encode.MakeTestOptions(desktop135P, videotype.VP8Prof),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8, caps.HWEncodeVP8OddDimension},
			ExtraData:         encode.TestData(desktop135P),
		}, {
			Name:              "vp8_180p",
			Val:               encode.MakeTestOptions(desktop180P, videotype.VP8Prof),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8},
			ExtraData:         encode.TestData(desktop180P),
		}, {
			Name:              "vp8_270p",
			Val:               encode.MakeTestOptions(desktop270P, videotype.VP8Prof),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8},
			ExtraData:         encode.TestData(desktop270P),
		}, {
			Name:              "vp8_360p",
			Val:               encode.MakeTestOptions(desktop360P, videotype.VP8Prof),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8},
			ExtraData:         encode.TestData(desktop360P),
		}, {
			Name:              "vp8_720p",
			Val:               encode.MakeTestOptions(desktop720P, videotype.VP8Prof),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8},
			ExtraData:         encode.TestData(desktop720P),
		}, {
			Name:              "vp8_1080p",
			Val:               encode.MakeTestOptions(desktop1080P, videotype.VP8Prof),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8},
			ExtraData:         encode.TestData(desktop1080P),
		}, {
			Name:              "vp8_1080p_global_vaapi_lock_disabled",
			Val:               encode.MakeTestOptionsWithNoGlobalVaapiLock(desktop1080P, videotype.VP8Prof),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8, "thread_safe_libva_backend"},
			ExtraData:         encode.TestData(desktop1080P),
		}, {
			Name:              "vp8_2160p",
			Val:               encode.MakeTestOptions(desktop2160P, videotype.VP8Prof),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8_4K},
			ExtraData:         encode.TestData(desktop2160P),
		}, {
			Name:              "vp8_720p_l1t2",
			Val:               encode.MakeTestOptionsWithSVCMode(desktop720P, videotype.VP8Prof, "L1T2"),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8, "vaapi"},
			ExtraData:         encode.TestData(desktop720P),
		}, {
			Name:              "vp8_720p_l1t3",
			Val:               encode.MakeTestOptionsWithSVCMode(desktop720P, videotype.VP8Prof, "L1T3"),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8, "vaapi"},
			ExtraData:         encode.TestData(desktop720P),
		}, {
			Name:              "vp8_1080p_l1t2",
			Val:               encode.MakeTestOptionsWithSVCMode(desktop1080P, videotype.VP8Prof, "L1T2"),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8, "vaapi"},
			ExtraData:         encode.TestData(desktop1080P),
		}, {
			Name:              "vp8_1080p_l1t3",
			Val:               encode.MakeTestOptionsWithSVCMode(desktop1080P, videotype.VP8Prof, "L1T3"),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8, "vaapi"},
			ExtraData:         encode.TestData(desktop1080P),
		}, {
			Name:              "vp9_180p",
			Val:               encode.MakeTestOptions(desktop180P, videotype.VP9Prof),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			ExtraData:         encode.TestData(desktop180P),
		}, {
			Name:              "vp9_135p",
			Val:               encode.MakeTestOptions(desktop135P, videotype.VP9Prof),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9, caps.HWEncodeVP9OddDimension},
			ExtraData:         encode.TestData(desktop135P),
		}, {
			Name:              "vp9_270p",
			Val:               encode.MakeTestOptions(desktop270P, videotype.VP9Prof),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			ExtraData:         encode.TestData(desktop270P),
		}, {
			Name:              "vp9_360p",
			Val:               encode.MakeTestOptions(desktop360P, videotype.VP9Prof),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			ExtraData:         encode.TestData(desktop360P),
		}, {
			Name:              "vp9_720p",
			Val:               encode.MakeTestOptions(desktop720P, videotype.VP9Prof),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			ExtraData:         encode.TestData(desktop720P),
		}, {
			Name:              "vp9_1080p",
			Val:               encode.MakeTestOptions(desktop1080P, videotype.VP9Prof),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			ExtraData:         encode.TestData(desktop1080P),
		}, {
			Name:              "vp9_1080p_global_vaapi_lock_disabled",
			Val:               encode.MakeTestOptionsWithNoGlobalVaapiLock(desktop1080P, videotype.VP9Prof),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9, "thread_safe_libva_backend"},
			ExtraData:         encode.TestData(desktop1080P),
		}, {
			Name:              "vp9_2160p",
			Val:               encode.MakeTestOptions(desktop2160P, videotype.VP9Prof),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9_4K},
			ExtraData:         encode.TestData(desktop2160P),
		}, {
			Name:              "vp9_720p_l1t2",
			Val:               encode.MakeTestOptionsWithSVCMode(desktop720P, videotype.VP9Prof, "L1T2"),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9, "vaapi"},
			ExtraData:         encode.TestData(desktop720P),
		}, {
			Name:              "vp9_720p_l1t3",
			Val:               encode.MakeTestOptionsWithSVCMode(desktop720P, videotype.VP9Prof, "L1T3"),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9, "vaapi"},
			ExtraData:         encode.TestData(desktop720P),
		}, {
			Name:              "vp9_540p_l2t3_key",
			Val:               encode.MakeTestOptionsWithSVCMode(desktop540P, videotype.VP9Prof, "L2T3_KEY"),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			ExtraData:         encode.TestData(desktop540P),
		}, {
			Name:              "vp9_540p_l3t3_key",
			Val:               encode.MakeTestOptionsWithSVCMode(desktop540P, videotype.VP9Prof, "L3T3_KEY"),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			ExtraData:         encode.TestData(desktop540P),
		}, {
			Name:              "vp9_720p_l2t3_key",
			Val:               encode.MakeTestOptionsWithSVCMode(desktop720P, videotype.VP9Prof, "L2T3_KEY"),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			ExtraData:         encode.TestData(desktop720P),
		}, {
			Name:              "vp9_720p_l3t3_key",
			Val:               encode.MakeTestOptionsWithSVCMode(desktop720P, videotype.VP9Prof, "L3T3_KEY"),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			ExtraData:         encode.TestData(desktop720P),
		}, {
			Name:              "av1_135p",
			Val:               encode.MakeTestOptions(desktop135P, videotype.AV1MainProf),
			ExtraData:         encode.TestData(desktop135P),
			ExtraSoftwareDeps: []string{caps.HWEncodeAV1, caps.HWEncodeAV1OddDimension},
		}, {
			Name:              "av1_180p",
			Val:               encode.MakeTestOptions(desktop180P, videotype.AV1MainProf),
			ExtraData:         encode.TestData(desktop180P),
			ExtraSoftwareDeps: []string{caps.HWEncodeAV1},
		}, {
			Name:              "av1_270p",
			Val:               encode.MakeTestOptions(desktop270P, videotype.AV1MainProf),
			ExtraData:         encode.TestData(desktop270P),
			ExtraSoftwareDeps: []string{caps.HWEncodeAV1},
		}, {
			Name:              "av1_360p",
			Val:               encode.MakeTestOptions(desktop360P, videotype.AV1MainProf),
			ExtraData:         encode.TestData(desktop360P),
			ExtraSoftwareDeps: []string{caps.HWEncodeAV1},
		}, {
			Name:              "av1_720p",
			Val:               encode.MakeTestOptions(desktop720P, videotype.AV1MainProf),
			ExtraData:         encode.TestData(desktop720P),
			ExtraSoftwareDeps: []string{caps.HWEncodeAV1},
		}, {
			Name:              "av1_1080p",
			Val:               encode.MakeTestOptions(desktop1080P, videotype.AV1MainProf),
			ExtraData:         encode.TestData(desktop1080P),
			ExtraSoftwareDeps: []string{caps.HWEncodeAV1},
		}, {
			Name:              "av1_2160p",
			Val:               encode.MakeTestOptions(desktop2160P, videotype.AV1MainProf),
			ExtraData:         encode.TestData(desktop2160P),
			ExtraSoftwareDeps: []string{caps.HWEncodeAV1_4K},
		}},
	})
}

func EncodeAccel(ctx context.Context, s *testing.State) {
	encode.RunAccelVideoTest(ctx, s, s.Param().(encode.TestOptions))
}
