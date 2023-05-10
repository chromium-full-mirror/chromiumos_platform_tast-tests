// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"context"
	"time"

	"chromiumos/tast/common/media/caps"
	"chromiumos/tast/local/bundles/cros/video/encode"
	"chromiumos/tast/local/media/videotype"
	"go.chromium.org/tast/core/testing"
)

const (
	tulip135P  = "tulip2-240x135.vp9.webm"
	tulip180P  = "tulip2-320x180.vp9.webm"
	tulip270P  = "tulip2-480x270.vp9.webm"
	tulip360P  = "tulip2-640x360.vp9.webm"
	tulip540P  = "tulip2-960x540.vp9.webm"
	tulip720P  = "tulip2-1280x720.vp9.webm"
	crowd1080P = "crowd-1920x1080.vp9.webm"
	crowd2160P = "crowd-3840x2160.vp9.webm"
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
			Val:               encode.MakeTestOptions(tulip180P, videotype.H264BaselineProf),
			ExtraSoftwareDeps: []string{caps.HWEncodeH264},
			ExtraData:         encode.TestData(tulip180P),
		}, {
			Name:              "h264_270p",
			Val:               encode.MakeTestOptions(tulip270P, videotype.H264BaselineProf),
			ExtraSoftwareDeps: []string{caps.HWEncodeH264},
			ExtraData:         encode.TestData(tulip270P),
		}, {
			Name:              "h264_360p",
			Val:               encode.MakeTestOptions(tulip360P, videotype.H264BaselineProf),
			ExtraSoftwareDeps: []string{caps.HWEncodeH264},
			ExtraData:         encode.TestData(tulip360P),
		}, {
			Name:              "h264_720p",
			Val:               encode.MakeTestOptions(tulip720P, videotype.H264BaselineProf),
			ExtraSoftwareDeps: []string{caps.HWEncodeH264},
			ExtraData:         encode.TestData(tulip720P),
		}, {
			Name:              "h264_720p_l1t2",
			Val:               encode.MakeTestOptionsWithSVCMode(tulip720P, videotype.H264BaselineProf, "L1T2"),
			ExtraSoftwareDeps: []string{caps.HWEncodeH264, "vaapi"},
			ExtraData:         encode.TestData(tulip720P),
		}, {
			Name:              "h264_720p_l1t3",
			Val:               encode.MakeTestOptionsWithSVCMode(tulip720P, videotype.H264BaselineProf, "L1T3"),
			ExtraSoftwareDeps: []string{caps.HWEncodeH264, "vaapi"},
			ExtraData:         encode.TestData(tulip720P),
		}, {
			Name:              "h264_720p_vbr",
			Val:               encode.MakeVBRTestOptions(tulip720P, videotype.H264BaselineProf),
			ExtraSoftwareDeps: []string{caps.HWEncodeH264VBR},
			ExtraData:         encode.TestData(tulip720P),
		}, {
			Name:              "h264_720p_l1t2_vbr",
			Val:               encode.MakeVBRTestOptionsWithSVCMode(tulip720P, videotype.H264BaselineProf, "L1T2"),
			ExtraSoftwareDeps: []string{caps.HWEncodeH264VBR, "vaapi"},
			ExtraData:         encode.TestData(tulip720P),
		}, {
			Name:              "h264_720p_l1t3_vbr",
			Val:               encode.MakeVBRTestOptionsWithSVCMode(tulip720P, videotype.H264BaselineProf, "L1T3"),
			ExtraSoftwareDeps: []string{caps.HWEncodeH264VBR, "vaapi"},
			ExtraData:         encode.TestData(tulip720P),
		}, {
			Name:              "h264_1080p",
			Val:               encode.MakeTestOptions(crowd1080P, videotype.H264BaselineProf),
			ExtraSoftwareDeps: []string{caps.HWEncodeH264},
			ExtraData:         encode.TestData(crowd1080P),
		}, {
			Name:              "h264_1080p_global_vaapi_lock_disabled",
			Val:               encode.MakeTestOptionsWithNoGlobalVaapiLock(crowd1080P, videotype.H264BaselineProf),
			ExtraSoftwareDeps: []string{caps.HWEncodeH264, "thread_safe_libva_backend"},
			ExtraData:         encode.TestData(crowd1080P),
		}, {
			Name:              "h264_2160p",
			Val:               encode.MakeTestOptions(crowd2160P, videotype.H264BaselineProf),
			ExtraSoftwareDeps: []string{caps.HWEncodeH264_4K},
			ExtraData:         encode.TestData(crowd2160P),
		}, {
			Name:              "vp8_135p",
			Val:               encode.MakeTestOptions(tulip135P, videotype.VP8Prof),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8, caps.HWEncodeVP8OddDimension},
			ExtraData:         encode.TestData(tulip135P),
		}, {
			Name:              "vp8_180p",
			Val:               encode.MakeTestOptions(tulip180P, videotype.VP8Prof),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8},
			ExtraData:         encode.TestData(tulip180P),
		}, {
			Name:              "vp8_270p",
			Val:               encode.MakeTestOptions(tulip270P, videotype.VP8Prof),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8},
			ExtraData:         encode.TestData(tulip270P),
		}, {
			Name:              "vp8_360p",
			Val:               encode.MakeTestOptions(tulip360P, videotype.VP8Prof),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8},
			ExtraData:         encode.TestData(tulip360P),
		}, {
			Name:              "vp8_720p",
			Val:               encode.MakeTestOptions(tulip720P, videotype.VP8Prof),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8},
			ExtraData:         encode.TestData(tulip720P),
		}, {
			Name:              "vp8_1080p",
			Val:               encode.MakeTestOptions(crowd1080P, videotype.VP8Prof),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8},
			ExtraData:         encode.TestData(crowd1080P),
		}, {
			Name:              "vp8_1080p_global_vaapi_lock_disabled",
			Val:               encode.MakeTestOptionsWithNoGlobalVaapiLock(crowd1080P, videotype.VP8Prof),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8, "thread_safe_libva_backend"},
			ExtraData:         encode.TestData(crowd1080P),
		}, {
			Name:              "vp8_2160p",
			Val:               encode.MakeTestOptions(crowd2160P, videotype.VP8Prof),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8_4K},
			ExtraData:         encode.TestData(crowd2160P),
		}, {
			Name:              "vp8_720p_l1t2",
			Val:               encode.MakeTestOptionsWithSVCMode(tulip720P, videotype.VP8Prof, "L1T2"),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8, "vaapi"},
			ExtraData:         encode.TestData(tulip720P),
		}, {
			Name:              "vp8_720p_l1t3",
			Val:               encode.MakeTestOptionsWithSVCMode(tulip720P, videotype.VP8Prof, "L1T3"),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8, "vaapi"},
			ExtraData:         encode.TestData(tulip720P),
		}, {
			Name:              "vp8_1080p_l1t2",
			Val:               encode.MakeTestOptionsWithSVCMode(crowd1080P, videotype.VP8Prof, "L1T2"),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8, "vaapi"},
			ExtraData:         encode.TestData(crowd1080P),
		}, {
			Name:              "vp8_1080p_l1t3",
			Val:               encode.MakeTestOptionsWithSVCMode(crowd1080P, videotype.VP8Prof, "L1T3"),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8, "vaapi"},
			ExtraData:         encode.TestData(crowd1080P),
		}, {
			Name:              "vp9_180p",
			Val:               encode.MakeTestOptions(tulip180P, videotype.VP9Prof),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			ExtraData:         encode.TestData(tulip180P),
		}, {
			Name:              "vp9_135p",
			Val:               encode.MakeTestOptions(tulip135P, videotype.VP9Prof),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9, caps.HWEncodeVP9OddDimension},
			ExtraData:         encode.TestData(tulip135P),
		}, {
			Name:              "vp9_270p",
			Val:               encode.MakeTestOptions(tulip270P, videotype.VP9Prof),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			ExtraData:         encode.TestData(tulip270P),
		}, {
			Name:              "vp9_360p",
			Val:               encode.MakeTestOptions(tulip360P, videotype.VP9Prof),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			ExtraData:         encode.TestData(tulip360P),
		}, {
			Name:              "vp9_720p",
			Val:               encode.MakeTestOptions(tulip720P, videotype.VP9Prof),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			ExtraData:         encode.TestData(tulip720P),
		}, {
			Name:              "vp9_1080p",
			Val:               encode.MakeTestOptions(crowd1080P, videotype.VP9Prof),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			ExtraData:         encode.TestData(crowd1080P),
		}, {
			Name:              "vp9_1080p_global_vaapi_lock_disabled",
			Val:               encode.MakeTestOptionsWithNoGlobalVaapiLock(crowd1080P, videotype.VP9Prof),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9, "thread_safe_libva_backend"},
			ExtraData:         encode.TestData(crowd1080P),
		}, {
			Name:              "vp9_2160p",
			Val:               encode.MakeTestOptions(crowd2160P, videotype.VP9Prof),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9_4K},
			ExtraData:         encode.TestData(crowd2160P),
		}, {
			Name:              "vp9_720p_l1t2",
			Val:               encode.MakeTestOptionsWithSVCMode(tulip720P, videotype.VP9Prof, "L1T2"),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9, "vaapi"},
			ExtraData:         encode.TestData(tulip720P),
		}, {
			Name:              "vp9_720p_l1t3",
			Val:               encode.MakeTestOptionsWithSVCMode(tulip720P, videotype.VP9Prof, "L1T3"),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9, "vaapi"},
			ExtraData:         encode.TestData(tulip720P),
		}, {
			Name:              "vp9_540p_l2t3_key",
			Val:               encode.MakeTestOptionsWithSVCMode(tulip540P, videotype.VP9Prof, "L2T3_KEY"),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			ExtraData:         encode.TestData(tulip540P),
		}, {
			Name:              "vp9_540p_l3t3_key",
			Val:               encode.MakeTestOptionsWithSVCMode(tulip540P, videotype.VP9Prof, "L3T3_KEY"),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			ExtraData:         encode.TestData(tulip540P),
		}, {
			Name:              "vp9_720p_l2t3_key",
			Val:               encode.MakeTestOptionsWithSVCMode(tulip720P, videotype.VP9Prof, "L2T3_KEY"),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			ExtraData:         encode.TestData(tulip720P),
		}, {
			Name:              "vp9_720p_l3t3_key",
			Val:               encode.MakeTestOptionsWithSVCMode(tulip720P, videotype.VP9Prof, "L3T3_KEY"),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			ExtraData:         encode.TestData(tulip720P),
		}, {
			Name:              "av1_135p",
			Val:               encode.MakeTestOptions(tulip135P, videotype.AV1MainProf),
			ExtraData:         encode.TestData(tulip135P),
			ExtraSoftwareDeps: []string{caps.HWEncodeAV1, caps.HWEncodeAV1OddDimension},
		}, {
			Name:              "av1_180p",
			Val:               encode.MakeTestOptions(tulip180P, videotype.AV1MainProf),
			ExtraData:         encode.TestData(tulip180P),
			ExtraSoftwareDeps: []string{caps.HWEncodeAV1},
		}, {
			Name:              "av1_270p",
			Val:               encode.MakeTestOptions(tulip270P, videotype.AV1MainProf),
			ExtraData:         encode.TestData(tulip270P),
			ExtraSoftwareDeps: []string{caps.HWEncodeAV1},
		}, {
			Name:              "av1_360p",
			Val:               encode.MakeTestOptions(tulip360P, videotype.AV1MainProf),
			ExtraData:         encode.TestData(tulip360P),
			ExtraSoftwareDeps: []string{caps.HWEncodeAV1},
		}, {
			Name:              "av1_720p",
			Val:               encode.MakeTestOptions(tulip720P, videotype.AV1MainProf),
			ExtraData:         encode.TestData(tulip720P),
			ExtraSoftwareDeps: []string{caps.HWEncodeAV1},
		}, {
			Name:              "av1_1080p",
			Val:               encode.MakeTestOptions(crowd1080P, videotype.AV1MainProf),
			ExtraData:         encode.TestData(crowd1080P),
			ExtraSoftwareDeps: []string{caps.HWEncodeAV1},
		}, {
			Name:              "av1_2160p",
			Val:               encode.MakeTestOptions(crowd2160P, videotype.AV1MainProf),
			ExtraData:         encode.TestData(crowd2160P),
			ExtraSoftwareDeps: []string{caps.HWEncodeAV1_4K},
		}},
	})
}

func EncodeAccel(ctx context.Context, s *testing.State) {
	encode.RunAccelVideoTest(ctx, s, s.Param().(encode.TestOptions))
}
