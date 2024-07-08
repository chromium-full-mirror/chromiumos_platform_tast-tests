// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"fmt"
	"testing"

	"go.chromium.org/tast-tests/cros/common/genparams"
)

// To regenerate the test parameters by running the following in a chroot:
// TAST_GENERATE_UPDATE=1 ~/chromiumos/src/platform/tast/tools/go.sh test -count=1 go.chromium.org/tast-tests/cros/local/bundles/cros/video

type paramData struct {
	Name         string
	SoftwareDeps string
	HardwareDeps string
	Data         []string
	Attr         []string
	Comment      string

	VideoFiles     string
	ValidatorType  string
	MustFail       bool
	IgnoredSysLogs string
}

// genFilesFromBugs generates multiple test cases for each files in the filesFromBugs map. The key of filesFromBugs would be appended in the test name and value will be assigned to VideoFiles.
func genFilesFromBugs(defaultParam paramData, filesFromBugs map[string]string) []paramData {
	var result []paramData
	for _, key := range sortedStringKeys(filesFromBugs) {
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
	params = append(params, []paramData{{
		Name:          "av1_common",
		Attr:          perBuildAttrs,
		SoftwareDeps:  "[]string{caps.HWDecodeAV1}",
		VideoFiles:    "av1Files[\"8bit\"]",
		ValidatorType: "decoding.MD5",
	}, {
		Name:          "av1_8bit_quantizer",
		Attr:          perBuildAttrs,
		SoftwareDeps:  "[]string{caps.HWDecodeAV1}",
		VideoFiles:    "av1Aom8bitFiles[\"quantizer\"]",
		ValidatorType: "decoding.MD5",
	}, {
		Name:          "av1_8bit_size_under_64x64",
		Attr:          perBuildAttrs,
		HardwareDeps:  "hwdep.D(hwdep.SkipCPUSocFamily(\"mediatek\"))",
		SoftwareDeps:  "[]string{caps.HWDecodeAV1}",
		VideoFiles:    "av1Aom8bitFiles[\"size_under_64x64\"]",
		ValidatorType: "decoding.MD5",
	}, {
		Name:          "av1_8bit_size",
		Attr:          perBuildAttrs,
		SoftwareDeps:  "[]string{caps.HWDecodeAV1}",
		VideoFiles:    "av1Aom8bitFiles[\"size\"]",
		ValidatorType: "decoding.MD5",
	}, {
		Name:          "av1_8bit_allintra",
		Attr:          perBuildAttrs,
		SoftwareDeps:  "[]string{caps.HWDecodeAV1}",
		VideoFiles:    "av1Aom8bitFiles[\"allintra\"]",
		ValidatorType: "decoding.MD5",
	}, {
		Name:          "av1_8bit_cdfupdate",
		Attr:          perBuildAttrs,
		SoftwareDeps:  "[]string{caps.HWDecodeAV1}",
		VideoFiles:    "av1Aom8bitFiles[\"cdfupdate\"]",
		ValidatorType: "decoding.MD5",
	}, {
		Name:          "av1_8bit_motionvec",
		Attr:          perBuildAttrs,
		SoftwareDeps:  "[]string{caps.HWDecodeAV1}",
		VideoFiles:    "av1Aom8bitFiles[\"motionvec\"]",
		ValidatorType: "decoding.MD5",
	}, {
		Name:          "av1_film_grain",
		Attr:          perBuildAttrs,
		SoftwareDeps:  "[]string{caps.HWDecodeAV1}",
		Comment:       "Different decoders may use different film grain synthesis methods while producing a visually correct output (AV1 spec 7.2). Thus we validate the decoding of film-grain streams using SSIM",
		VideoFiles:    "av1FilmGrainFiles",
		ValidatorType: "decoding.SSIM",
	}, {
		Name:          "av1_10bit_common",
		Attr:          perBuildAttrs,
		SoftwareDeps:  "[]string{caps.HWDecodeAV1_10BPP}",
		VideoFiles:    "av1Files[\"10bit\"]",
		ValidatorType: "decoding.MD5",
	}, {
		Name:          "av1_10bit_common_quantizer",
		Attr:          perBuildAttrs,
		SoftwareDeps:  "[]string{caps.HWDecodeAV1_10BPP}",
		VideoFiles:    "av1Files[\"10bit_quantizer\"]",
		ValidatorType: "decoding.MD5",
	}, {
		Name:          "av1_10bit_film_grain",
		Attr:          perBuildAttrs,
		SoftwareDeps:  "[]string{caps.HWDecodeAV1_10BPP}",
		Comment:       "Different decoders may use different film grain synthesis methods while producing a visually correct output (AV1 spec 7.2). Thus, don't use the decoding.MD5 validator and use decoding.SSIM instead",
		VideoFiles:    "av110BitFilmGrainFiles",
		ValidatorType: "decoding.SSIM",
	}, {
		Name:           "h264_invalid_bitstreams",
		Attr:           perBuildAttrs,
		SoftwareDeps:   "[]string{caps.HWDecodeH264, \"proprietary_codecs\"}",
		VideoFiles:     "h264InvalidBitstreams",
		ValidatorType:  "decoding.MD5",
		MustFail:       true,
		IgnoredSysLogs: "graphics.SysLogMediatekVideoErrors",
	}, {
		Name:          "h264_baseline",
		Attr:          perBuildAttrs,
		SoftwareDeps:  "[]string{caps.HWDecodeH264, \"proprietary_codecs\"}",
		VideoFiles:    "h264Files[\"baseline\"]",
		ValidatorType: "decoding.MD5",
	}, {
		Name:          "h264_main",
		Attr:          perBuildAttrs,
		SoftwareDeps:  "[]string{caps.HWDecodeH264, \"proprietary_codecs\"}",
		VideoFiles:    "h264Files[\"main\"]",
		ValidatorType: "decoding.MD5",
	}, {
		Name:          "h264_high",
		Attr:          perBuildAttrs,
		SoftwareDeps:  "[]string{caps.HWDecodeH264, \"proprietary_codecs\"}",
		VideoFiles:    "h264Files[\"high\"]",
		ValidatorType: "decoding.MD5",
	}, {
		Name:          "h264_first_mb_in_slice",
		Attr:          perBuildAttrs,
		HardwareDeps:  "hwdep.D(hwdep.SupportsV4L2StatefulVideoDecoding())",
		SoftwareDeps:  "[]string{caps.HWDecodeH264, \"proprietary_codecs\"}",
		VideoFiles:    "h264Files[\"first_mb_in_slice\"]",
		ValidatorType: "decoding.MD5",
	}, {
		Name:          "vp8_comprehensive",
		Attr:          perBuildAttrs,
		SoftwareDeps:  "[]string{caps.HWDecodeVP8}",
		VideoFiles:    "vp8Files[\"comprehensive\"]",
		ValidatorType: "decoding.MD5",
	}, {
		Name:          "vp8_inter",
		Attr:          perBuildAttrs,
		SoftwareDeps:  "[]string{caps.HWDecodeVP8}",
		VideoFiles:    "vp8Files[\"inter\"]",
		ValidatorType: "decoding.MD5",
	}, {
		Name:          "vp8_inter_multi_coeff",
		Attr:          perBuildAttrs,
		SoftwareDeps:  "[]string{caps.HWDecodeVP8}",
		VideoFiles:    "vp8Files[\"inter_multi_coeff\"]",
		ValidatorType: "decoding.MD5",
	}, {
		Name:          "vp8_inter_segment",
		Attr:          perBuildAttrs,
		SoftwareDeps:  "[]string{caps.HWDecodeVP8}",
		VideoFiles:    "vp8Files[\"inter_segment\"]",
		ValidatorType: "decoding.MD5",
	}, {
		Name:          "vp8_intra",
		Attr:          perBuildAttrs,
		SoftwareDeps:  "[]string{caps.HWDecodeVP8}",
		VideoFiles:    "vp8Files[\"intra\"]",
		ValidatorType: "decoding.MD5",
	}, {
		Name:          "vp8_intra_multi_coeff",
		Attr:          perBuildAttrs,
		SoftwareDeps:  "[]string{caps.HWDecodeVP8}",
		VideoFiles:    "vp8Files[\"intra_multi_coeff\"]",
		ValidatorType: "decoding.MD5",
	}, {
		Name:          "vp8_intra_segment",
		Attr:          perBuildAttrs,
		SoftwareDeps:  "[]string{caps.HWDecodeVP8}",
		VideoFiles:    "vp8Files[\"intra_segment\"]",
		ValidatorType: "decoding.MD5",
	}, {
		Name:          "vp9_0_group1_buf",
		Attr:          perBuildAttrs,
		SoftwareDeps:  "[]string{caps.HWDecodeVP9}",
		VideoFiles:    "vp9WebmFiles[\"profile_0\"][\"group1\"][\"buf\"]",
		ValidatorType: "decoding.MD5",
	}, {
		Name:          "vp9_0_group1_frm_resize",
		Attr:          perBuildAttrs,
		HardwareDeps:  "hwdep.D(hwdep.SkipOnV4L2StatelessVideoDecoding())",
		SoftwareDeps:  "[]string{caps.HWDecodeVP9}",
		VideoFiles:    "vp9WebmFiles[\"profile_0\"][\"group1\"][\"frm_resize\"]",
		ValidatorType: "decoding.MD5",
	}, {
		Name:          "vp9_0_group1_gf_dist",
		Attr:          perBuildAttrs,
		SoftwareDeps:  "[]string{caps.HWDecodeVP9}",
		VideoFiles:    "vp9WebmFiles[\"profile_0\"][\"group1\"][\"gf_dist\"]",
		ValidatorType: "decoding.MD5",
	}, {
		Name:          "vp9_0_group1_odd_size",
		Attr:          perBuildAttrs,
		SoftwareDeps:  "[]string{caps.HWDecodeVP9}",
		VideoFiles:    "vp9WebmFiles[\"profile_0\"][\"group1\"][\"odd_size\"]",
		ValidatorType: "decoding.MD5",
	}, {
		Name:          "vp9_0_group1_sub8x8",
		Attr:          perBuildAttrs,
		SoftwareDeps:  "[]string{caps.HWDecodeVP9}",
		VideoFiles:    "vp9WebmFiles[\"profile_0\"][\"group1\"][\"sub8x8\"]",
		ValidatorType: "decoding.MD5",
	}, {
		Name:          "vp9_0_group1_sub8x8_sf",
		Attr:          perBuildAttrs,
		HardwareDeps:  "hwdep.D(hwdep.SkipOnV4L2StatelessVideoDecoding())",
		SoftwareDeps:  "[]string{caps.HWDecodeVP9}",
		VideoFiles:    "vp9WebmFiles[\"profile_0\"][\"group1\"][\"sub8x8_sf\"]",
		ValidatorType: "decoding.MD5",
	}, {
		Name:          "vp9_2_group1_buf",
		Attr:          perBuildAttrs,
		SoftwareDeps:  "[]string{caps.HWDecodeVP9_2}",
		VideoFiles:    "vp9WebmFiles[\"profile_2\"][\"group1\"][\"buf\"]",
		ValidatorType: "decoding.MD5",
	}, {
		Name:          "vp9_2_group1_frm_resize",
		Attr:          perBuildAttrs,
		HardwareDeps:  "hwdep.D(hwdep.SkipOnV4L2StatelessVideoDecoding())",
		SoftwareDeps:  "[]string{caps.HWDecodeVP9_2}",
		VideoFiles:    "vp9WebmFiles[\"profile_2\"][\"group1\"][\"frm_resize\"]",
		ValidatorType: "decoding.MD5",
	}, {
		Name:          "vp9_2_group1_gf_dist",
		Attr:          perBuildAttrs,
		SoftwareDeps:  "[]string{caps.HWDecodeVP9_2}",
		VideoFiles:    "vp9WebmFiles[\"profile_2\"][\"group1\"][\"gf_dist\"]",
		ValidatorType: "decoding.MD5",
	}, {
		Name:          "vp9_2_group1_odd_size",
		Attr:          perBuildAttrs,
		SoftwareDeps:  "[]string{caps.HWDecodeVP9_2}",
		VideoFiles:    "vp9WebmFiles[\"profile_2\"][\"group1\"][\"odd_size\"]",
		ValidatorType: "decoding.MD5",
	}, {
		Name:          "vp9_2_group1_sub8x8",
		Attr:          perBuildAttrs,
		SoftwareDeps:  "[]string{caps.HWDecodeVP9_2}",
		VideoFiles:    "vp9WebmFiles[\"profile_2\"][\"group1\"][\"sub8x8\"]",
		ValidatorType: "decoding.MD5",
	}, {
		Name:          "vp9_2_group1_sub8x8_sf",
		Attr:          perBuildAttrs,
		HardwareDeps:  "hwdep.D(hwdep.SkipOnV4L2StatelessVideoDecoding())",
		SoftwareDeps:  "[]string{caps.HWDecodeVP9_2}",
		VideoFiles:    "vp9WebmFiles[\"profile_2\"][\"group1\"][\"sub8x8_sf\"]",
		ValidatorType: "decoding.MD5",
	}, {
		Name: "vp9_0_svc",
		// TODO(b/210167476): Reenable when it's not failing everywhere.
		//Attr:         perBuildAttrs,
		HardwareDeps:  "hwdep.D(hwdep.SkipOnV4L2StatelessVideoDecoding())",
		SoftwareDeps:  "[]string{caps.HWDecodeVP9}",
		VideoFiles:    "vp9SVCFiles",
		ValidatorType: "decoding.MD5",
	}, {
		Name:          "hevc_main_part_1",
		Attr:          perBuildAttrs,
		SoftwareDeps:  "[]string{caps.HWDecodeHEVC}",
		VideoFiles:    "hevcFiles[\"main_part_1\"]",
		ValidatorType: "decoding.MD5",
	}, {
		Name:          "hevc_main_part_2",
		Attr:          perBuildAttrs,
		SoftwareDeps:  "[]string{caps.HWDecodeHEVC}",
		VideoFiles:    "hevcFiles[\"main_part_2\"]",
		ValidatorType: "decoding.MD5",
	}, {
		Name:          "hevc_main_part_3",
		Attr:          perBuildAttrs,
		SoftwareDeps:  "[]string{caps.HWDecodeHEVC}",
		VideoFiles:    "hevcFiles[\"main_part_3\"]",
		ValidatorType: "decoding.MD5",
	}, {
		Name:          "hevc_main_part_4",
		Attr:          perBuildAttrs,
		SoftwareDeps:  "[]string{caps.HWDecodeHEVC}",
		VideoFiles:    "hevcFiles[\"main_part_4\"]",
		ValidatorType: "decoding.MD5",
	},
	}...)

	// Generate test cases for each |files_from_bugs| for ease of triaging.
	params = append(params, genFilesFromBugs(paramData{
		Name:          "h264_files_from_bugs",
		Attr:          perBuildAttrs,
		SoftwareDeps:  "[]string{caps.HWDecodeH264, \"proprietary_codecs\"}",
		ValidatorType: "decoding.MD5",
	}, h264FilesFromBugs)...)
	params = append(params, genFilesFromBugs(paramData{
		Name:          "h264_4k_files_from_bugs",
		Attr:          perBuildAttrs,
		SoftwareDeps:  "[]string{caps.HWDecodeH264_4K, \"proprietary_codecs\"}",
		ValidatorType: "decoding.MD5",
	}, h2644kFilesFromBugs)...)
	params = append(params, genFilesFromBugs(paramData{
		Name:          "vp9_files_from_bugs",
		Attr:          perBuildAttrs,
		SoftwareDeps:  "[]string{caps.HWDecodeVP9}",
		ValidatorType: "decoding.MD5",
	}, vp9FilesFromBugs)...)
	params = append(params, genFilesFromBugs(paramData{
		Name:          "av1_files_from_bugs",
		Attr:          perBuildAttrs,
		SoftwareDeps:  "[]string{caps.HWDecodeAV1}",
		ValidatorType: "decoding.MD5",
	}, av1FilesFromBugs)...)
	params = append(params, genFilesFromBugs(paramData{
		Name:          "hevc_files_from_bugs",
		Attr:          perBuildAttrs,
		SoftwareDeps:  "[]string{caps.HWDecodeHEVC, \"proprietary_codecs\"}",
		ValidatorType: "decoding.MD5",
	}, h265FilesFromBugs)...)

	for _, bugID := range sortedStringKeys(hevcFilesFromBugs) {
		params = append(params, []paramData{{
			Name:          fmt.Sprintf("hevc_files_from_bugs_%s", bugID),
			Attr:          perBuildAttrs,
			SoftwareDeps:  "[]string{caps.HWDecodeHEVC, \"proprietary_codecs\"}",
			VideoFiles:    fmt.Sprintf("hevcFilesFromBugs[\"%s\"]", bugID),
			ValidatorType: "decoding.MD5",
		}}...)
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
						{{ if .IgnoredSysLogs }}
						ignoredSysLogs: []graphics.SysLogCategory{ {{ .IgnoredSysLogs }} },
						{{ end }}

		},
	},
	{{ end }}`, params)
	genparams.Ensure(t, "chrome_stack_decoder_verification.go", code)
}
