// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"sort"
	"testing"

	"go.chromium.org/tast-tests/cros/common/genparams"
)

// To regenerate the test parameters by running the following in a chroot:
// TAST_GENERATE_UPDATE=1 ~/chromiumos/src/platform/tast/tools/go.sh test -count=1 go.chromium.org/tast-tests/cros/local/bundles/cros/video
var h264FilesFromBugs = map[string]string{
	"149068426":   "test_vectors/h264/files_from_bugs/b_149068426_invalid_video_layout_mtk_8183_with_direct_videodecoder.h264",
	"172838252":   "test_vectors/h264/files_from_bugs/b_172838252_pixelated_video_on_rk3399.h264",
	"174733646":   "test_vectors/h264/files_from_bugs/b_174733646_video_with_out_of_order_frames_mtk_8173.h264",
	"210895987":   "test_vectors/h264/files_from_bugs/b_210895987_still-colors-360p.h264",
	"277849540_1": "test_vectors/h264/files_from_bugs/b_277849540__malformed_h264_vlct_16x16.h264",
	"277849540_2": "test_vectors/h264/files_from_bugs/b_277849540__malformed_h264_vlct_4x4.h264",
	"277849540_3": "test_vectors/h264/files_from_bugs/b_277849540__malformed_h264_vlct_8x8.h264",
	"276358257":   "test_vectors/h264/files_from_bugs/b_276358257__amd_gpu_gen3_lockup.h264",
	"299320432":   "test_vectors/h264/files_from_bugs/b_299320432__amd_skyrim_system_hang.h264",
}

var h2644kFilesFromBugs = map[string]string{
	"22704778": "test_vectors/h264/files_from_bugs/b_227047778_mtk_8195_artifacts.h264",
}

var vp9FilesFromBugs = map[string]string{
	"177839888": "test_vectors/vp9/files_from_bugs/b_177839888__rk3399_vp9_artifacts_with_video_decoder_japanews24.ivf",
	"251040563": "test_vectors/vp9/files_from_bugs/b_251040563_webrtc_libvpx.vp9.ivf",
}

var av1FilesFromBugs = map[string]string{
	"235138734": "test_vectors/av1/files_from_bugs/b_235138734_test-25fps-one-to-four-tiles.av1.ivf",
}

var h265FilesFromBugs = map[string]string{
	"321622872": "test_vectors/hevc/files_from_bugs/b_321622872__bands_across_screen_4k.hevc",
}

type paramData struct {
	Name         string
	SoftwareDeps string
	HardwareDeps string
	Data         []string
	Attr         []string
	Comment      string

	VideoFiles      string
	ValidatorType   string
	EnabledFeatures []string
	MustFail        bool
}

// genFilesFromBugs generates multiple test cases for each files in the filesFromBugs map. The key of filesFromBugs would be appended in the test name and value will be assigned to VideoFiles.
func genFilesFromBugs(defaultParam paramData, filesFromBugs map[string]string) []paramData {
	var result []paramData
	// Iterate the map in order
	keys := make([]string, 0)
	for k := range filesFromBugs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		data := defaultParam
		data.Name = data.Name + "_" + key
		data.VideoFiles = "[]string{\"" + filesFromBugs[key] + "\"}"
		result = append(result, data)
	}
	return result
}

func TestChromeStackDecoderVerificationParams(t *testing.T) {
	perBuildAttrs := []string{"group:graphics", "graphics_video", "graphics_perbuild", "graphics_video_chromestackdecoding"}
	params := []paramData{
		{
			Name:          "av1_common",
			Attr:          perBuildAttrs,
			SoftwareDeps:  `[]string{caps.HWDecodeAV1}`,
			VideoFiles:    "av1CommonFiles",
			ValidatorType: "decoding.MD5",
		}, {
			Name:          "av1_8bit_quantizer",
			Attr:          perBuildAttrs,
			SoftwareDeps:  `[]string{caps.HWDecodeAV1}`,
			VideoFiles:    "av1Aom8bitFiles[\"quantizer\"]",
			ValidatorType: "decoding.MD5",
		}, {
			Name:          "av1_8bit_size",
			Attr:          perBuildAttrs,
			SoftwareDeps:  `[]string{caps.HWDecodeAV1}`,
			VideoFiles:    "av1Aom8bitFiles[\"size\"]",
			ValidatorType: "decoding.MD5",
		}, {
			Name:          "av1_8bit_allintra",
			Attr:          perBuildAttrs,
			SoftwareDeps:  `[]string{caps.HWDecodeAV1}`,
			VideoFiles:    "av1Aom8bitFiles[\"allintra\"]",
			ValidatorType: "decoding.MD5",
		}, {
			Name:          "av1_8bit_cdfupdate",
			Attr:          perBuildAttrs,
			SoftwareDeps:  `[]string{caps.HWDecodeAV1}`,
			VideoFiles:    "av1Aom8bitFiles[\"cdfupdate\"]",
			ValidatorType: "decoding.MD5",
		}, {
			Name:          "av1_8bit_motionvec",
			Attr:          perBuildAttrs,
			SoftwareDeps:  `[]string{caps.HWDecodeAV1}`,
			VideoFiles:    "av1Aom8bitFiles[\"motionvec\"]",
			ValidatorType: "decoding.MD5",
		}, {
			Name:          "av1_film_grain",
			Attr:          perBuildAttrs,
			SoftwareDeps:  `[]string{caps.HWDecodeAV1}`,
			Comment:       "Different decoders may use different film grain synthesis methods while producing a visually correct output (AV1 spec 7.2). Thus we validate the decoding of film-grain streams using SSIM.",
			VideoFiles:    "av1FilmGrainFiles",
			ValidatorType: "decoding.SSIM",
		}, {
			Name:          "av1_10bit_common",
			Attr:          perBuildAttrs,
			SoftwareDeps:  `[]string{caps.HWDecodeAV1_10BPP}`,
			VideoFiles:    "av110BitCommonFiles",
			ValidatorType: "decoding.MD5",
		}, {
			Name:          "av1_10bit_film_grain",
			Attr:          perBuildAttrs,
			SoftwareDeps:  `[]string{caps.HWDecodeAV1_10BPP}`,
			Comment:       "Different decoders may use different film grain synthesis methods while producing a visually correct output (AV1 spec 7.2). Thus, for volteer, don't validate the decoding of film-grain streams using MD5. Instead, validate them using SSIM (see the av1_10bit_ssim test).",
			VideoFiles:    "av110BitFilmGrainFiles",
			ValidatorType: "decoding.SSIM",
		}, {
			Name:          "h264_invalid_bitstreams",
			Attr:          perBuildAttrs,
			HardwareDeps:  "hwdep.D(hwdep.SkipGPUFamily(\"rogue\"))",
			SoftwareDeps:  `[]string{caps.HWDecodeH264, "proprietary_codecs"}`,
			VideoFiles:    "h264InvalidBitstreams",
			ValidatorType: "decoding.MD5",
			MustFail:      true,
		}, {
			Name:          "h264_baseline",
			Attr:          perBuildAttrs,
			HardwareDeps:  "hwdep.D(hwdep.SkipGPUFamily(\"rogue\"))",
			SoftwareDeps:  `[]string{caps.HWDecodeH264, "proprietary_codecs"}`,
			VideoFiles:    "h264Files[\"baseline\"]",
			ValidatorType: "decoding.MD5",
		}, {
			Name:          "h264_main",
			Attr:          perBuildAttrs,
			HardwareDeps:  "hwdep.D(hwdep.SkipGPUFamily(\"rogue\"))",
			SoftwareDeps:  `[]string{caps.HWDecodeH264, "proprietary_codecs"}`,
			VideoFiles:    "h264Files[\"main\"]",
			ValidatorType: "decoding.MD5",
		}, {
			Name:          "h264_high",
			Attr:          perBuildAttrs,
			HardwareDeps:  "hwdep.D(hwdep.SkipGPUFamily(\"rogue\"))",
			SoftwareDeps:  `[]string{caps.HWDecodeH264, "proprietary_codecs"}`,
			VideoFiles:    "h264Files[\"high\"]",
			ValidatorType: "decoding.MD5",
		}, {
			Name:          "h264_first_mb_in_slice",
			Attr:          perBuildAttrs,
			HardwareDeps:  "hwdep.D(hwdep.SupportsV4L2StatefulVideoDecoding(), hwdep.SkipGPUFamily(\"rogue\"))",
			SoftwareDeps:  `[]string{caps.HWDecodeH264, "proprietary_codecs"}`,
			VideoFiles:    "h264Files[\"first_mb_in_slice\"]",
			ValidatorType: "decoding.MD5",
		}, {
			Name:          "vp8_comprehensive",
			Attr:          perBuildAttrs,
			HardwareDeps:  "hwdep.D(hwdep.SkipGPUFamily(\"rogue\"))",
			SoftwareDeps:  `[]string{caps.HWDecodeVP8}`,
			VideoFiles:    "vp8ComprehensiveFiles",
			ValidatorType: "decoding.MD5",
		}, {
			Name:          "vp8_inter",
			Attr:          perBuildAttrs,
			HardwareDeps:  "hwdep.D(hwdep.SkipGPUFamily(\"rogue\"))",
			SoftwareDeps:  `[]string{caps.HWDecodeVP8}`,
			VideoFiles:    "vp8InterFiles",
			ValidatorType: "decoding.MD5",
		}, {
			Name:          "vp8_inter_multi_coeff",
			Attr:          perBuildAttrs,
			HardwareDeps:  "hwdep.D(hwdep.SkipGPUFamily(\"rogue\"))",
			SoftwareDeps:  `[]string{caps.HWDecodeVP8}`,
			VideoFiles:    "vp8InterMultiCoeffFiles",
			ValidatorType: "decoding.MD5",
		}, {
			Name:          "vp8_inter_segment",
			Attr:          perBuildAttrs,
			HardwareDeps:  "hwdep.D(hwdep.SkipGPUFamily(\"rogue\"))",
			SoftwareDeps:  `[]string{caps.HWDecodeVP8}`,
			VideoFiles:    "vp8InterSegmentFiles",
			ValidatorType: "decoding.MD5",
		}, {
			Name:          "vp8_intra",
			Attr:          perBuildAttrs,
			HardwareDeps:  "hwdep.D(hwdep.SkipGPUFamily(\"rogue\"))",
			SoftwareDeps:  `[]string{caps.HWDecodeVP8}`,
			VideoFiles:    "vp8IntraFiles",
			ValidatorType: "decoding.MD5",
		}, {
			Name:          "vp8_intra_multi_coeff",
			Attr:          perBuildAttrs,
			HardwareDeps:  "hwdep.D(hwdep.SkipGPUFamily(\"rogue\"))",
			SoftwareDeps:  `[]string{caps.HWDecodeVP8}`,
			VideoFiles:    "vp8IntraMultiCoeffSegmentFiles",
			ValidatorType: "decoding.MD5",
		}, {
			Name:          "vp8_intra_segment",
			Attr:          perBuildAttrs,
			HardwareDeps:  "hwdep.D(hwdep.SkipGPUFamily(\"rogue\"))",
			SoftwareDeps:  `[]string{caps.HWDecodeVP8}`,
			VideoFiles:    "vp8IntraSegmentFiles",
			ValidatorType: "decoding.MD5",
		}, {
			Name:          "vp9_0_group1_buf",
			Attr:          perBuildAttrs,
			HardwareDeps:  "hwdep.D(hwdep.SkipGPUFamily(\"rogue\"))",
			SoftwareDeps:  `[]string{caps.HWDecodeVP9}`,
			VideoFiles:    "vp90Group1Buf",
			ValidatorType: "decoding.MD5",
		}, {
			Name:          "vp9_0_group1_frm_resize",
			Attr:          perBuildAttrs,
			HardwareDeps:  "hwdep.D(hwdep.SkipOnV4L2StatelessVideoDecoding(), hwdep.SkipGPUFamily(\"rogue\"))",
			SoftwareDeps:  `[]string{caps.HWDecodeVP9}`,
			VideoFiles:    "vp90Group1FrmResize",
			ValidatorType: "decoding.MD5",
		}, {
			Name:          "vp9_0_group1_gf_dist",
			Attr:          perBuildAttrs,
			HardwareDeps:  "hwdep.D(hwdep.SkipGPUFamily(\"rogue\"))",
			SoftwareDeps:  `[]string{caps.HWDecodeVP9}`,
			VideoFiles:    "vp90Group1GfDist",
			ValidatorType: "decoding.MD5",
		}, {
			Name:          "vp9_0_group1_odd_size",
			Attr:          perBuildAttrs,
			HardwareDeps:  "hwdep.D(hwdep.SkipGPUFamily(\"rogue\"))",
			SoftwareDeps:  `[]string{caps.HWDecodeVP9}`,
			VideoFiles:    "vp90Group1OddSize",
			ValidatorType: "decoding.MD5",
		}, {
			Name:          "vp9_0_group1_sub8x8",
			Attr:          perBuildAttrs,
			HardwareDeps:  "hwdep.D(hwdep.SkipGPUFamily(\"rogue\"))",
			SoftwareDeps:  `[]string{caps.HWDecodeVP9}`,
			VideoFiles:    "vp90Group1Sub8x8",
			ValidatorType: "decoding.MD5",
		}, {
			Name:          "vp9_0_group1_sub8x8_sf",
			Attr:          perBuildAttrs,
			HardwareDeps:  "hwdep.D(hwdep.SkipOnV4L2StatelessVideoDecoding(), hwdep.SkipGPUFamily(\"rogue\"))",
			SoftwareDeps:  `[]string{caps.HWDecodeVP9}`,
			VideoFiles:    "vp90Group1Sub8x8Sf",
			ValidatorType: "decoding.MD5",
		}, {
			Name:          "vp9_2_group1_buf",
			Attr:          perBuildAttrs,
			HardwareDeps:  "hwdep.D(hwdep.SkipGPUFamily(\"rogue\"))",
			SoftwareDeps:  `[]string{caps.HWDecodeVP9_2}`,
			VideoFiles:    "vp92Group1Buf",
			ValidatorType: "decoding.MD5",
		}, {
			Name:          "vp9_2_group1_frm_resize",
			Attr:          perBuildAttrs,
			HardwareDeps:  "hwdep.D(hwdep.SkipGPUFamily(\"rogue\"))",
			SoftwareDeps:  `[]string{caps.HWDecodeVP9_2}`,
			VideoFiles:    "vp92Group1FrmResize",
			ValidatorType: "decoding.MD5",
		}, {
			Name:          "vp9_2_group1_gf_dist",
			Attr:          perBuildAttrs,
			HardwareDeps:  "hwdep.D(hwdep.SkipGPUFamily(\"rogue\"))",
			SoftwareDeps:  `[]string{caps.HWDecodeVP9_2}`,
			VideoFiles:    "vp92Group1GfDist",
			ValidatorType: "decoding.MD5",
		}, {
			Name:          "vp9_2_group1_odd_size",
			Attr:          perBuildAttrs,
			HardwareDeps:  "hwdep.D(hwdep.SkipGPUFamily(\"rogue\"))",
			SoftwareDeps:  `[]string{caps.HWDecodeVP9_2}`,
			VideoFiles:    "vp92Group1OddSize",
			ValidatorType: "decoding.MD5",
		}, {
			Name:          "vp9_2_group1_sub8x8",
			Attr:          perBuildAttrs,
			HardwareDeps:  "hwdep.D(hwdep.SkipGPUFamily(\"rogue\"))",
			SoftwareDeps:  `[]string{caps.HWDecodeVP9_2}`,
			VideoFiles:    "vp92Group1Sub8x8",
			ValidatorType: "decoding.MD5",
		}, {
			Name:          "vp9_2_group1_sub8x8_sf",
			Attr:          perBuildAttrs,
			HardwareDeps:  "hwdep.D(hwdep.SkipGPUFamily(\"rogue\"))",
			SoftwareDeps:  `[]string{caps.HWDecodeVP9_2}`,
			VideoFiles:    "vp92Group1Sub8x8Sf",
			ValidatorType: "decoding.MD5",
		}, {
			Name: "vp9_0_svc",
			// TODO(b/210167476): Reenable when it's not failing everywhere.
			//Attr:         perBuildAttrs,
			HardwareDeps:  "hwdep.D(hwdep.SkipGPUFamily(\"rogue\"))",
			SoftwareDeps:  `[]string{caps.HWDecodeVP9}`,
			VideoFiles:    "vp9SVCFiles",
			ValidatorType: "decoding.MD5",
		}, {
			Name:          "hevc_main_part_1",
			Attr:          perBuildAttrs,
			HardwareDeps:  "hwdep.D(hwdep.SkipGPUFamily(\"rogue\"))",
			SoftwareDeps:  `[]string{caps.HWDecodeHEVC}`,
			VideoFiles:    "hevcCommonFiles[\"1\"]",
			ValidatorType: "decoding.MD5",
		}, {
			Name:          "hevc_main_part_2",
			Attr:          perBuildAttrs,
			HardwareDeps:  "hwdep.D(hwdep.SkipGPUFamily(\"rogue\"))",
			SoftwareDeps:  `[]string{caps.HWDecodeHEVC}`,
			VideoFiles:    "hevcCommonFiles[\"2\"]",
			ValidatorType: "decoding.MD5",
		}, {
			Name:          "hevc_main_part_3",
			Attr:          perBuildAttrs,
			HardwareDeps:  "hwdep.D(hwdep.SkipGPUFamily(\"rogue\"))",
			SoftwareDeps:  `[]string{caps.HWDecodeHEVC}`,
			VideoFiles:    "hevcCommonFiles[\"3\"]",
			ValidatorType: "decoding.MD5",
		}, {
			Name:          "hevc_main_part_4",
			Attr:          perBuildAttrs,
			HardwareDeps:  "hwdep.D(hwdep.SkipGPUFamily(\"rogue\"))",
			SoftwareDeps:  `[]string{caps.HWDecodeHEVC}`,
			VideoFiles:    "hevcCommonFiles[\"4\"]",
			ValidatorType: "decoding.MD5",
		},
	}

	// generate test case for each files_from_bugs so that we can easily find
	// a decoder fails decoding a specific bug file.
	params = append(params, genFilesFromBugs(paramData{
		Name:          "h264_files_from_bugs",
		Attr:          perBuildAttrs,
		HardwareDeps:  "hwdep.D(hwdep.SkipGPUFamily(\"rogue\"))",
		SoftwareDeps:  `[]string{caps.HWDecodeH264, "proprietary_codecs"}`,
		ValidatorType: "decoding.MD5",
	}, h264FilesFromBugs)...)
	params = append(params, genFilesFromBugs(paramData{
		Name:          "h264_4k_files_from_bugs",
		Attr:          perBuildAttrs,
		HardwareDeps:  "hwdep.D(hwdep.SkipGPUFamily(\"rogue\"))",
		SoftwareDeps:  `[]string{caps.HWDecodeH264_4K, "proprietary_codecs"}`,
		ValidatorType: "decoding.MD5",
	}, h2644kFilesFromBugs)...)
	params = append(params, genFilesFromBugs(paramData{
		Name:          "vp9_files_from_bugs",
		Attr:          perBuildAttrs,
		HardwareDeps:  "hwdep.D(hwdep.SkipGPUFamily(\"rogue\"))",
		SoftwareDeps:  `[]string{caps.HWDecodeVP9}`,
		ValidatorType: "decoding.MD5",
	}, vp9FilesFromBugs)...)
	params = append(params, genFilesFromBugs(paramData{
		Name:          "av1_files_from_bugs",
		Attr:          perBuildAttrs,
		HardwareDeps:  "hwdep.D(hwdep.SkipGPUFamily(\"rogue\"))",
		SoftwareDeps:  `[]string{caps.HWDecodeAV1}`,
		ValidatorType: "decoding.MD5",
	}, av1FilesFromBugs)...)
	params = append(params, genFilesFromBugs(paramData{
		Name:          "hevc_files_from_bugs",
		Attr:          perBuildAttrs,
		HardwareDeps:  "hwdep.D(hwdep.SkipGPUFamily(\"rogue\"))",
		SoftwareDeps:  `[]string{caps.HWDecodeHEVC, "proprietary_codecs"}`,
		ValidatorType: "decoding.MD5",
	}, h265FilesFromBugs)...)

	// V4L2FlatStatefulVideoDecoder VPx tests.
	params = append(params, []paramData{{
		Name:            "v4l2_flat_vp8_comprehensive",
		Attr:            perBuildAttrs,
		SoftwareDeps:    `[]string{caps.HWDecodeVP8, "v4l2_codec"}`,
		HardwareDeps:    "hwdep.D(hwdep.SupportsV4L2FlatVideoDecoding())",
		VideoFiles:      "vp8ComprehensiveFiles",
		ValidatorType:   "decoding.MD5",
		EnabledFeatures: []string{"V4L2FlatStatefulVideoDecoder"},
	}, {
		Name:            "v4l2_flat_vp8_inter",
		Attr:            perBuildAttrs,
		SoftwareDeps:    `[]string{caps.HWDecodeVP8, "v4l2_codec"}`,
		HardwareDeps:    "hwdep.D(hwdep.SupportsV4L2FlatVideoDecoding())",
		VideoFiles:      "vp8InterFiles",
		ValidatorType:   "decoding.MD5",
		EnabledFeatures: []string{"V4L2FlatStatefulVideoDecoder"},
	}, {
		Name:            "v4l2_flat_vp8_inter_multi_coeff",
		Attr:            perBuildAttrs,
		SoftwareDeps:    `[]string{caps.HWDecodeVP8, "v4l2_codec"}`,
		HardwareDeps:    "hwdep.D(hwdep.SupportsV4L2FlatVideoDecoding())",
		VideoFiles:      "vp8InterMultiCoeffFiles",
		ValidatorType:   "decoding.MD5",
		EnabledFeatures: []string{"V4L2FlatStatefulVideoDecoder"},
	}, {
		Name:            "v4l2_flat_vp8_inter_segment",
		Attr:            perBuildAttrs,
		SoftwareDeps:    `[]string{caps.HWDecodeVP8, "v4l2_codec"}`,
		HardwareDeps:    "hwdep.D(hwdep.SupportsV4L2FlatVideoDecoding())",
		VideoFiles:      "vp8InterSegmentFiles",
		ValidatorType:   "decoding.MD5",
		EnabledFeatures: []string{"V4L2FlatStatefulVideoDecoder"},
	}, {
		Name:            "v4l2_flat_vp8_intra",
		Attr:            perBuildAttrs,
		SoftwareDeps:    `[]string{caps.HWDecodeVP8, "v4l2_codec"}`,
		HardwareDeps:    "hwdep.D(hwdep.SupportsV4L2FlatVideoDecoding())",
		VideoFiles:      "vp8IntraFiles",
		ValidatorType:   "decoding.MD5",
		EnabledFeatures: []string{"V4L2FlatStatefulVideoDecoder"},
	}, {
		Name:            "v4l2_flat_vp8_intra_multi_coeff",
		Attr:            perBuildAttrs,
		SoftwareDeps:    `[]string{caps.HWDecodeVP8, "v4l2_codec"}`,
		HardwareDeps:    "hwdep.D(hwdep.SupportsV4L2FlatVideoDecoding())",
		VideoFiles:      "vp8IntraMultiCoeffSegmentFiles",
		ValidatorType:   "decoding.MD5",
		EnabledFeatures: []string{"V4L2FlatStatefulVideoDecoder"},
	}, {
		Name:            "v4l2_flat_vp8_intra_segment",
		Attr:            perBuildAttrs,
		SoftwareDeps:    `[]string{caps.HWDecodeVP8, "v4l2_codec"}`,
		HardwareDeps:    "hwdep.D(hwdep.SupportsV4L2FlatVideoDecoding())",
		VideoFiles:      "vp8IntraSegmentFiles",
		ValidatorType:   "decoding.MD5",
		EnabledFeatures: []string{"V4L2FlatStatefulVideoDecoder"},
	}, {
		Name:            "v4l2_flat_vp9_0_group1_buf",
		Attr:            perBuildAttrs,
		SoftwareDeps:    `[]string{caps.HWDecodeVP9, "v4l2_codec"}`,
		HardwareDeps:    "hwdep.D(hwdep.SupportsV4L2FlatVideoDecoding())",
		VideoFiles:      "vp90Group1Buf",
		ValidatorType:   "decoding.MD5",
		EnabledFeatures: []string{"V4L2FlatStatefulVideoDecoder"},
	}, {
		Name:            "v4l2_flat_vp9_0_group1_gf_dist",
		Attr:            perBuildAttrs,
		SoftwareDeps:    `[]string{caps.HWDecodeVP9, "v4l2_codec"}`,
		HardwareDeps:    "hwdep.D(hwdep.SupportsV4L2FlatVideoDecoding())",
		VideoFiles:      "vp90Group1GfDist",
		ValidatorType:   "decoding.MD5",
		EnabledFeatures: []string{"V4L2FlatStatefulVideoDecoder"},
	}, {
		Name:            "v4l2_flat_vp9_0_group1_odd_size",
		Attr:            perBuildAttrs,
		SoftwareDeps:    `[]string{caps.HWDecodeVP9, "v4l2_codec"}`,
		HardwareDeps:    "hwdep.D(hwdep.SupportsV4L2FlatVideoDecoding())",
		VideoFiles:      "vp90Group1OddSize",
		ValidatorType:   "decoding.MD5",
		EnabledFeatures: []string{"V4L2FlatStatefulVideoDecoder"},
	}, {
		Name:            "v4l2_flat_vp9_0_group1_sub8x8",
		Attr:            perBuildAttrs,
		SoftwareDeps:    `[]string{caps.HWDecodeVP9, "v4l2_codec"}`,
		HardwareDeps:    "hwdep.D(hwdep.SupportsV4L2FlatVideoDecoding())",
		VideoFiles:      "vp90Group1Sub8x8",
		ValidatorType:   "decoding.MD5",
		EnabledFeatures: []string{"V4L2FlatStatefulVideoDecoder"},
	}, {
		Name:            "v4l2_flat_h264_main",
		Attr:            perBuildAttrs,
		SoftwareDeps:    `[]string{caps.HWDecodeH264, "v4l2_codec"}`,
		HardwareDeps:    "hwdep.D(hwdep.SupportsV4L2FlatVideoDecoding())",
		VideoFiles:      "h264Files[\"main\"]",
		ValidatorType:   "decoding.MD5",
		EnabledFeatures: []string{"V4L2FlatStatefulVideoDecoder"},
	}, {
		Name:            "v4l2_flat_h264_baseline",
		Attr:            perBuildAttrs,
		SoftwareDeps:    `[]string{caps.HWDecodeH264, "v4l2_codec"}`,
		HardwareDeps:    "hwdep.D(hwdep.SupportsV4L2FlatVideoDecoding())",
		VideoFiles:      "h264Files[\"baseline\"]",
		ValidatorType:   "decoding.MD5",
		EnabledFeatures: []string{"V4L2FlatStatefulVideoDecoder"},
	}, {
		Name:            "v4l2_flat_h264_high",
		Attr:            perBuildAttrs,
		SoftwareDeps:    `[]string{caps.HWDecodeH264, "v4l2_codec"}`,
		HardwareDeps:    "hwdep.D(hwdep.SupportsV4L2FlatVideoDecoding())",
		VideoFiles:      "h264Files[\"high\"]",
		ValidatorType:   "decoding.MD5",
		EnabledFeatures: []string{"V4L2FlatStatefulVideoDecoder"},
	}, {
		Name:            "v4l2_flat_h264_first_mb_in_slice",
		Attr:            perBuildAttrs,
		SoftwareDeps:    `[]string{caps.HWDecodeH264, "v4l2_codec"}`,
		HardwareDeps:    "hwdep.D(hwdep.SupportsV4L2FlatVideoDecoding())",
		VideoFiles:      "h264Files[\"first_mb_in_slice\"]",
		ValidatorType:   "decoding.MD5",
		EnabledFeatures: []string{"V4L2FlatStatefulVideoDecoder"},
	},
	// TODO(b/189500115): Add v4l2_flat_p9_0_group1_frm_resize/sub8x8_sf.
	}...)

	code := genparams.Template(t, `{{ range . }}{
		Name: {{ .Name | fmt }},
        {{ if .Comment }}
        // {{ .Comment }}
        {{ end }}
		{{ if .Attr }}
		ExtraAttr: {{ .Attr | fmt }},
		{{ end }}
		{{ if .HardwareDeps }}
		ExtraHardwareDeps: {{ .HardwareDeps }},
		{{ end }}
		{{ if .SoftwareDeps }}
		ExtraSoftwareDeps: {{ .SoftwareDeps }},
		{{ end }}
		ExtraData: appendJSONFiles({{ .VideoFiles }}),
		Timeout: calculateTestTimeout({{ .VideoFiles }}, {{ .Name | fmt }}),
		Val:  chromeStackDecoderVerificationTestParam{
            videoFiles: {{ .VideoFiles  }},
            validatorType: {{ .ValidatorType }},
            mustFail: {{ .MustFail | fmt }},
            enabledFeatures: {{ .EnabledFeatures | fmt }},
		},
	},
	{{ end }}`, params)
	genparams.Ensure(t, "chrome_stack_decoder_verification.go", code)
}
