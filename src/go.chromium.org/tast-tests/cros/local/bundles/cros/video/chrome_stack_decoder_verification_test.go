// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"go.chromium.org/tast-tests/cros/common/genparams"
)

// To regenerate the test parameters by running the following in a chroot:
// TAST_GENERATE_UPDATE=1 ~/chromiumos/src/platform/tast/tools/go.sh test -count=1 go.chromium.org/tast-tests/cros/local/bundles/cros/video
var h264FilesFromBugs = map[string]string{
	"149068426": "test_vectors/h264/files_from_bugs/b_149068426_invalid_video_layout_mtk_8183_with_direct_videodecoder.h264",
	"172838252": "test_vectors/h264/files_from_bugs/b_172838252_pixelated_video_on_rk3399.h264",
	"174733646": "test_vectors/h264/files_from_bugs/b_174733646_video_with_out_of_order_frames_mtk_8173.h264",
	"210895987": "test_vectors/h264/files_from_bugs/b_210895987_still-colors-360p.h264",
	"276358257": "test_vectors/h264/files_from_bugs/b_276358257__amd_gpu_gen3_lockup.h264",
	"299320432": "test_vectors/h264/files_from_bugs/b_299320432__amd_skyrim_system_hang.h264",
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
	"346405213": "test_vectors/av1/files_from_bugs/b_346405213_reference-frame-scaling-test.ivf",
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
	IgnoredSysLogs  string
}

func genCombinedDeps(format string, deps []string) string {
	allKeys := make(map[string]bool)
	var dedupedDeps []string
	for _, item := range deps {
		if _, value := allKeys[item]; !value {
			allKeys[item] = true
			dedupedDeps = append(dedupedDeps, item)
		}
	}

	dependencies := strings.Join(dedupedDeps, ", ")
	if len(dependencies) > 0 {
		return fmt.Sprintf(format, dependencies)
	}
	return ""
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
	var params []paramData

	runs := map[string]paramData{
		"": paramData{
			HardwareDeps: "hwdep.SkipGPUFamily(\"rogue\")",
		},
		"v4l2_flat_": paramData{
			SoftwareDeps:    "\"v4l2_codec\"",
			EnabledFeatures: []string{"V4L2FlatStatefulVideoDecoder"},
			HardwareDeps:    "hwdep.SupportsV4L2FlatVideoDecoding()",
		},
	}

	// Sort the keys because map traversal is not deterministic.
	runsKeys := make([]string, 0, len(runs))
	for k := range runs {
		runsKeys = append(runsKeys, k)
	}
	sort.Strings(runsKeys)

	for _, testGroup := range runsKeys {
		param := runs[testGroup]
		params = append(params, []paramData{{
			Name:            fmt.Sprintf("%sav1_common", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeAV1", param.SoftwareDeps}),
			VideoFiles:      "av1Files[\"8bit\"]",
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, {
			Name:            fmt.Sprintf("%sav1_8bit_quantizer", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeAV1", param.SoftwareDeps}),
			VideoFiles:      "av1Aom8bitFiles[\"quantizer\"]",
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, {
			Name:            fmt.Sprintf("%sav1_8bit_size_under_64x64", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{"hwdep.SkipCPUSocFamily(\"mediatek\")", param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeAV1", param.SoftwareDeps}),
			VideoFiles:      "av1Aom8bitFiles[\"size_under_64x64\"]",
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, {
			Name:            fmt.Sprintf("%sav1_8bit_size", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeAV1", param.SoftwareDeps}),
			VideoFiles:      "av1Aom8bitFiles[\"size\"]",
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, {
			Name:            fmt.Sprintf("%sav1_8bit_allintra", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeAV1", param.SoftwareDeps}),
			VideoFiles:      "av1Aom8bitFiles[\"allintra\"]",
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, {
			Name:            fmt.Sprintf("%sav1_8bit_cdfupdate", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeAV1", param.SoftwareDeps}),
			VideoFiles:      "av1Aom8bitFiles[\"cdfupdate\"]",
			EnabledFeatures: param.EnabledFeatures,
			ValidatorType:   "decoding.MD5",
		}, {
			Name:            fmt.Sprintf("%sav1_8bit_motionvec", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeAV1", param.SoftwareDeps}),
			VideoFiles:      "av1Aom8bitFiles[\"motionvec\"]",
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, {
			Name:            fmt.Sprintf("%sav1_film_grain", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeAV1", param.SoftwareDeps}),
			Comment:         "Different decoders may use different film grain synthesis methods while producing a visually correct output (AV1 spec 7.2). Thus we validate the decoding of film-grain streams using SSIM.",
			VideoFiles:      "av1FilmGrainFiles",
			ValidatorType:   "decoding.SSIM",
			EnabledFeatures: param.EnabledFeatures,
		}, {
			Name:            fmt.Sprintf("%sav1_10bit_common", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeAV1_10BPP", param.SoftwareDeps}),
			VideoFiles:      "av1Files[\"10bit\"]",
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, {
			Name:            fmt.Sprintf("%sav1_10bit_common_quantizer", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeAV1_10BPP", param.SoftwareDeps}),
			VideoFiles:      "av1Files[\"10bit_quantizer\"]",
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, {
			Name:            fmt.Sprintf("%sav1_10bit_film_grain", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeAV1_10BPP", param.SoftwareDeps}),
			Comment:         "Different decoders may use different film grain synthesis methods while producing a visually correct output (AV1 spec 7.2). Thus, for volteer, don't validate the decoding of film-grain streams using MD5. Instead, validate them using SSIM (see the av1_10bit_ssim test).",
			VideoFiles:      "av110BitFilmGrainFiles",
			ValidatorType:   "decoding.SSIM",
			EnabledFeatures: param.EnabledFeatures,
		}, {
			Name:            fmt.Sprintf("%sh264_invalid_bitstreams", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeH264", "\"proprietary_codecs\"", param.SoftwareDeps}),
			VideoFiles:      "h264InvalidBitstreams",
			ValidatorType:   "decoding.MD5",
			MustFail:        true,
			EnabledFeatures: param.EnabledFeatures,
			IgnoredSysLogs:  "graphics.SysLogMediatekVideoErrors",
		}, {
			Name:            fmt.Sprintf("%sh264_baseline", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeH264", "\"proprietary_codecs\"", param.SoftwareDeps}),
			VideoFiles:      "h264Files[\"baseline\"]",
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, {
			Name:            fmt.Sprintf("%sh264_main", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeH264", "\"proprietary_codecs\"", param.SoftwareDeps}),
			VideoFiles:      "h264Files[\"main\"]",
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, {
			Name:            fmt.Sprintf("%sh264_high", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeH264", "\"proprietary_codecs\"", param.SoftwareDeps}),
			VideoFiles:      "h264Files[\"high\"]",
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, {
			Name:            fmt.Sprintf("%sh264_first_mb_in_slice", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{"hwdep.SupportsV4L2StatefulVideoDecoding()", param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeH264", "\"proprietary_codecs\"", param.SoftwareDeps}),
			VideoFiles:      "h264Files[\"first_mb_in_slice\"]",
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, {
			Name:            fmt.Sprintf("%svp8_comprehensive", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeVP8", param.SoftwareDeps}),
			VideoFiles:      "vp8ComprehensiveFiles",
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, {
			Name:            fmt.Sprintf("%svp8_inter", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeVP8", param.SoftwareDeps}),
			VideoFiles:      "vp8InterFiles",
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, {
			Name:            fmt.Sprintf("%svp8_inter_multi_coeff", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeVP8", param.SoftwareDeps}),
			VideoFiles:      "vp8InterMultiCoeffFiles",
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, {
			Name:            fmt.Sprintf("%svp8_inter_segment", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeVP8", param.SoftwareDeps}),
			VideoFiles:      "vp8InterSegmentFiles",
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, {
			Name:            fmt.Sprintf("%svp8_intra", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeVP8", param.SoftwareDeps}),
			VideoFiles:      "vp8IntraFiles",
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, {
			Name:            fmt.Sprintf("%svp8_intra_multi_coeff", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeVP8", param.SoftwareDeps}),
			VideoFiles:      "vp8IntraMultiCoeffSegmentFiles",
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, {
			Name:            fmt.Sprintf("%svp8_intra_segment", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeVP8", param.SoftwareDeps}),
			VideoFiles:      "vp8IntraSegmentFiles",
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, {
			Name:            fmt.Sprintf("%svp9_0_group1_buf", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeVP9", param.SoftwareDeps}),
			VideoFiles:      "vp90Group1Buf",
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, {
			Name:            fmt.Sprintf("%svp9_0_group1_frm_resize", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{"hwdep.SkipOnV4L2StatelessVideoDecoding()", param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeVP9", param.SoftwareDeps}),
			VideoFiles:      "vp90Group1FrmResize",
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, {
			Name:            fmt.Sprintf("%svp9_0_group1_gf_dist", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeVP9", param.SoftwareDeps}),
			VideoFiles:      "vp90Group1GfDist",
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, {
			Name:            fmt.Sprintf("%svp9_0_group1_odd_size", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeVP9", param.SoftwareDeps}),
			VideoFiles:      "vp90Group1OddSize",
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, {
			Name:            fmt.Sprintf("%svp9_0_group1_sub8x8", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeVP9", param.SoftwareDeps}),
			VideoFiles:      "vp90Group1Sub8x8",
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, {
			Name:            fmt.Sprintf("%svp9_0_group1_sub8x8_sf", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{"hwdep.SkipOnV4L2StatelessVideoDecoding()", param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeVP9", param.SoftwareDeps}),
			VideoFiles:      "vp90Group1Sub8x8Sf",
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, {
			Name:            fmt.Sprintf("%svp9_2_group1_buf", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeVP9_2", param.SoftwareDeps}),
			VideoFiles:      "vp92Group1Buf",
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, {
			Name:            fmt.Sprintf("%svp9_2_group1_frm_resize", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{"hwdep.SkipOnV4L2StatelessVideoDecoding()", param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeVP9_2", param.SoftwareDeps}),
			VideoFiles:      "vp92Group1FrmResize",
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, {
			Name:            fmt.Sprintf("%svp9_2_group1_gf_dist", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeVP9_2", param.SoftwareDeps}),
			VideoFiles:      "vp92Group1GfDist",
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, {
			Name:            fmt.Sprintf("%svp9_2_group1_odd_size", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeVP9_2", param.SoftwareDeps}),
			VideoFiles:      "vp92Group1OddSize",
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, {
			Name:            fmt.Sprintf("%svp9_2_group1_sub8x8", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeVP9_2", param.SoftwareDeps}),
			VideoFiles:      "vp92Group1Sub8x8",
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, {
			Name:            fmt.Sprintf("%svp9_2_group1_sub8x8_sf", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{"hwdep.SkipOnV4L2StatelessVideoDecoding()", param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeVP9_2", param.SoftwareDeps}),
			VideoFiles:      "vp92Group1Sub8x8Sf",
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, {
			Name: fmt.Sprintf("%svp9_0_svc", testGroup),
			// TODO(b/210167476): Reenable when it's not failing everywhere.
			//Attr:         perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{"hwdep.SkipOnV4L2StatelessVideoDecoding()", param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeVP9", param.SoftwareDeps}),
			VideoFiles:      "vp9SVCFiles",
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, {
			Name:            fmt.Sprintf("%shevc_main_part_1", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeHEVC", param.SoftwareDeps}),
			VideoFiles:      "hevcCommonFiles[\"1\"]",
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, {
			Name:            fmt.Sprintf("%shevc_main_part_2", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeHEVC", param.SoftwareDeps}),
			VideoFiles:      "hevcCommonFiles[\"2\"]",
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, {
			Name:            fmt.Sprintf("%shevc_main_part_3", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeHEVC", param.SoftwareDeps}),
			VideoFiles:      "hevcCommonFiles[\"3\"]",
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, {
			Name:            fmt.Sprintf("%shevc_main_part_4", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeHEVC", param.SoftwareDeps}),
			VideoFiles:      "hevcCommonFiles[\"4\"]",
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		},
		}...)
		// generate test case for each files_from_bugs so that we can easily find
		// a decoder fails decoding a specific bug file.
		params = append(params, genFilesFromBugs(paramData{
			Name:            fmt.Sprintf("%sh264_files_from_bugs", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{"hwdep.SkipGPUFamily(\"rogue\")", param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeH264", "\"proprietary_codecs\"", param.SoftwareDeps}),
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, h264FilesFromBugs)...)
		params = append(params, genFilesFromBugs(paramData{
			Name:            fmt.Sprintf("%sh264_4k_files_from_bugs", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{"hwdep.SkipGPUFamily(\"rogue\")", param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeH264_4K", "\"proprietary_codecs\"", param.SoftwareDeps}),
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, h2644kFilesFromBugs)...)
		params = append(params, genFilesFromBugs(paramData{
			Name:            fmt.Sprintf("%svp9_files_from_bugs", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{"hwdep.SkipGPUFamily(\"rogue\")", param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeVP9", param.SoftwareDeps}),
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, vp9FilesFromBugs)...)
		params = append(params, genFilesFromBugs(paramData{
			Name:            fmt.Sprintf("%sav1_files_from_bugs", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{"hwdep.SkipGPUFamily(\"rogue\")", param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeAV1", param.SoftwareDeps}),
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, av1FilesFromBugs)...)
		params = append(params, genFilesFromBugs(paramData{
			Name:            fmt.Sprintf("%shevc_files_from_bugs", testGroup),
			Attr:            perBuildAttrs,
			HardwareDeps:    genCombinedDeps("hwdep.D(%s)", []string{"hwdep.SkipGPUFamily(\"rogue\")", param.HardwareDeps}),
			SoftwareDeps:    genCombinedDeps("[]string{%s}", []string{"caps.HWDecodeHEVC", "\"proprietary_codecs\"", param.SoftwareDeps}),
			ValidatorType:   "decoding.MD5",
			EnabledFeatures: param.EnabledFeatures,
		}, h265FilesFromBugs)...)
	}

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
		ExtraSoftwareDeps:    {{ .SoftwareDeps }},
		{{ end }}
		ExtraData: appendJSONFiles({{ .VideoFiles }}),
		Timeout: calculateTestTimeout({{ .VideoFiles }}, {{ .Name | fmt }}),
		Val:  chromeStackDecoderVerificationTestParam{
						videoFiles: {{ .VideoFiles  }},
						validatorType: {{ .ValidatorType }},
						mustFail: {{ .MustFail | fmt }},
						enabledFeatures: {{ .EnabledFeatures | fmt }},
						{{ if .IgnoredSysLogs }}
						ignoredSysLogs: []graphics.SysLogCategory{ {{ .IgnoredSysLogs }} },
						{{ end }}

		},
	},
	{{ end }}`, params)
	genparams.Ensure(t, "chrome_stack_decoder_verification.go", code)
}
