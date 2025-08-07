// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.chromium.org/tast-tests/cros/common/genparams"
	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/video/test_vectors"
	"go.chromium.org/tast-tests/cros/local/chrome"
)

const ffmpegMD5Path = "/usr/local/graphics/ffmpeg_md5sum"
const ccdecPath = "ccdec"

// NB: If modifying any of the files or test specifications, be sure to
// regenerate the test parameters by running the following in a chroot:
// TAST_GENERATE_UPDATE=1 ~/chromiumos/src/platform/tast/tools/go.sh test -count=1 go.chromium.org/tast-tests/cros/local/bundles/cros/video

func genDecoderArgsBuilder(prefix, codec string) string {
	if strings.Contains(prefix, "ffmpeg") {
		return "platform.FFMPEGMD5DecodeVAAPIArgs"
	}

	ret := "platform."

	if codec == "vp8" {
		ret += "VP8"
	}
	if codec == "vp9" {
		ret += "VP9"
	}
	if codec == "h264" {
		ret += "H264"
	}
	if codec == "h265" || codec == "hevc" {
		ret += "HEVC"
	}
	if codec == "av1" {
		ret += "AV1"
	}

	ret += "Decode"

	if strings.Contains(prefix, "cros_codecs") {
		ret += "CrosCodecs"
	} else {
		ret += "VAAPI"
	}

	ret += "args"

	return ret
}

func TestPlatformDecodingParams(t *testing.T) {
	type paramData struct {
		Name               string
		Decoder            string
		DecoderArgsBuilder string
		Files              string
		IgnoredSysLogs     string
		Timeout            time.Duration
		HardwareDeps       string
		SoftwareDeps       []string
		Metadata           string
		Attr               []string
	}

	var params []paramData

	// Define timeouts, with extensions for specific groups.
	const defaultTimeout = 10 * time.Minute
	vp9GroupExtensions := map[string]time.Duration{
		"group4":   24 * time.Hour,
		"level5_0": 24 * time.Hour,
		"level5_1": 24 * time.Hour,
	}

	vaapiTestParams := []struct {
		binaryName     string
		testNamePrefix string
		frequency      string
	}{
		{filepath.Join(chrome.BinTestDir, "decode_test"), "", "graphics_perbuild"},
		{ccdecPath, "cros_codecs_", "graphics_perbuild"},
		{ffmpegMD5Path, "ffmpeg_", "graphics_nightly"},
	}

	for _, vaapiTestParam := range vaapiTestParams {
		// Generate VAAPI VP9 tests.
		for i, profile := range []string{"profile_0"} {
			for _, levelGroup := range []string{"group1", "group2", "group3", "group4", "level5_0", "level5_1"} {
				for _, cat := range []string{
					"buf", "frm_resize", "gf_dist", "odd_size", "sub8x8", "sub8x8_sf",
				} {
					files := fmt.Sprintf("test_vectors.VP9WebmFiles[\"%s\"][\"%s\"][\"%s\"]", profile, levelGroup, cat)
					param := paramData{
						Name:               fmt.Sprintf("%svaapi_vp9_%d_%s_%s", vaapiTestParam.testNamePrefix, i, levelGroup, cat),
						Decoder:            vaapiTestParam.binaryName,
						DecoderArgsBuilder: genDecoderArgsBuilder(vaapiTestParam.testNamePrefix, "vp9"),
						Files:              files,
						Timeout:            defaultTimeout,
						SoftwareDeps:       []string{"vaapi"},
						Metadata:           files,
						Attr:               []string{"graphics_video_vp9", vaapiTestParam.frequency},
					}
					if extension, ok := vp9GroupExtensions[levelGroup]; ok {
						param.Timeout = extension
					}

					var hardwareDeps []string

					// TODO(b/184683272): Reenable everywhere.
					if cat == "frm_resize" || cat == "sub8x8_sf" {
						// TODO(b/436839084): Reenable when supported.
						if vaapiTestParam.testNamePrefix == "cros_codecs_" {
							continue
						}
						hardwareDeps = append(hardwareDeps, "hwdep.SkipGPUFamily(\"picasso\")")
					}

					if vaapiTestParam.testNamePrefix == "cros_codecs_" {
						hardwareDeps = append(hardwareDeps, "hwdep.SupportsCrosCodecs()")
					}

					switch levelGroup {
					case "level5_0":
						param.SoftwareDeps = append(param.SoftwareDeps, caps.HWDecodeVP9_4K)
					case "level5_1":
						param.SoftwareDeps = append(param.SoftwareDeps, caps.HWDecodeVP9_4K60)
						hardwareDeps = append(hardwareDeps, "hwdep.MinMemory(7169)")
					default:
						param.SoftwareDeps = append(param.SoftwareDeps, caps.HWDecodeVP9)
					}

					param.HardwareDeps = strings.Join(hardwareDeps, ", ")
					params = append(params, param)
				}
			}
		}

		// Generate VAAPI AV1 tests.
		param := paramData{
			Name:               fmt.Sprintf("%svaapi_av1", vaapiTestParam.testNamePrefix),
			Decoder:            vaapiTestParam.binaryName,
			DecoderArgsBuilder: genDecoderArgsBuilder(vaapiTestParam.testNamePrefix, "av1"),
			Files:              "test_vectors.AV1Files[\"8bit\"]",
			Timeout:            defaultTimeout,
			// These SoftwareDeps do not include the 10 bit version of AV1.
			SoftwareDeps: []string{"vaapi", caps.HWDecodeAV1},
			Metadata:     "test_vectors.AV1Files[\"8bit\"]",
			Attr:         []string{"graphics_video_av1", "graphics_perbuild"},
		}
		if vaapiTestParam.testNamePrefix == "cros_codecs_" {
			param.HardwareDeps = "hwdep.SupportsCrosCodecs()"
		}
		params = append(params, param)
		for _, cat := range []string{"quantizer", "size", "allintra", "cdfupdate", "motionvec"} {
			files := fmt.Sprintf("test_vectors.AV1Aom8bitFiles[\"%s\"]", cat)
			param := paramData{
				Name:               fmt.Sprintf("%svaapi_av1_8bit_%s", vaapiTestParam.testNamePrefix, cat),
				Decoder:            vaapiTestParam.binaryName,
				DecoderArgsBuilder: genDecoderArgsBuilder(vaapiTestParam.testNamePrefix, "av1"),
				Files:              files,
				Timeout:            defaultTimeout,
				// These SoftwareDeps do not include the 10 bit version of AV1.
				SoftwareDeps: []string{"vaapi", caps.HWDecodeAV1},
				Metadata:     files,
				Attr:         []string{"graphics_video_av1", "graphics_perbuild"},
			}
			if vaapiTestParam.testNamePrefix == "cros_codecs_" {
				param.HardwareDeps = "hwdep.SupportsCrosCodecs()"
			}
			params = append(params, param)
		}

		// Generate VAAPI HEVC tests.
		for _, testGroup := range []string{"main_part_1", "main_part_2", "main_part_3", "main_part_4"} {
			files := fmt.Sprintf("test_vectors.HEVCFiles[\"%s\"]", testGroup)

			param := paramData{
				Name:               fmt.Sprintf("%svaapi_hevc_%s", vaapiTestParam.testNamePrefix, testGroup),
				Decoder:            vaapiTestParam.binaryName,
				DecoderArgsBuilder: genDecoderArgsBuilder(vaapiTestParam.testNamePrefix, "h265"),
				Files:              files,
				Timeout:            defaultTimeout,
				SoftwareDeps:       []string{"vaapi", caps.HWDecodeHEVC},
				Metadata:           files,
				Attr:               []string{"graphics_video_hevc", vaapiTestParam.frequency},
			}
			if vaapiTestParam.testNamePrefix == "cros_codecs_" {
				param.HardwareDeps = "hwdep.SupportsCrosCodecs()"
			}
			params = append(params, param)
		}
		param = paramData{
			Name:               fmt.Sprintf("%shevc_main_part_5_8k", vaapiTestParam.testNamePrefix),
			Decoder:            vaapiTestParam.binaryName,
			DecoderArgsBuilder: genDecoderArgsBuilder(vaapiTestParam.testNamePrefix, "hevc"),
			Files:              "test_vectors.HEVCFiles[\"main_part_5_8K\"]",
			Timeout:            defaultTimeout,
			SoftwareDeps:       []string{"vaapi", caps.HWDecodeHEVC8K},
			Metadata:           "test_vectors.HEVCFiles[\"main_part_5_8K\"]",
			Attr:               []string{"graphics_video_hevc", vaapiTestParam.frequency},
		}
		if vaapiTestParam.testNamePrefix == "cros_codecs_" {
			param.HardwareDeps = "hwdep.SupportsCrosCodecs()"
		}
		params = append(params, param)

		// Generate VAAPI VP8 tests.
		for _, testGroup := range []string{"inter", "inter_multi_coeff", "inter_segment", "intra", "intra_multi_coeff", "intra_segment", "comprehensive"} {
			files := fmt.Sprintf("test_vectors.VP8Files[\"%s\"]", testGroup)

			param := paramData{
				Name:               fmt.Sprintf("%svaapi_vp8_%s", vaapiTestParam.testNamePrefix, testGroup),
				Decoder:            vaapiTestParam.binaryName,
				DecoderArgsBuilder: genDecoderArgsBuilder(vaapiTestParam.testNamePrefix, "vp8"),
				Files:              files,
				Timeout:            defaultTimeout,
				SoftwareDeps:       []string{"vaapi", caps.HWDecodeVP8},
				Metadata:           files,
				Attr:               []string{"graphics_video_vp8", vaapiTestParam.frequency},
			}
			if vaapiTestParam.testNamePrefix == "cros_codecs_" {
				param.HardwareDeps = "hwdep.SupportsCrosCodecs()"
			}
			params = append(params, param)
		}

		// Generates VAAPI H264 tests.
		for _, group := range []string{"baseline", "main", "first_mb_in_slice"} {
			files := fmt.Sprintf("test_vectors.H264Files[\"%s\"]", group)

			param := paramData{
				Name:               fmt.Sprintf("%svaapi_h264_%s", vaapiTestParam.testNamePrefix, group),
				Decoder:            vaapiTestParam.binaryName,
				DecoderArgsBuilder: genDecoderArgsBuilder(vaapiTestParam.testNamePrefix, "h264"),
				Files:              files,
				Timeout:            defaultTimeout,
				SoftwareDeps:       []string{"vaapi", caps.HWDecodeH264},
				Metadata:           files,
				Attr:               []string{"graphics_video_h264", vaapiTestParam.frequency},
			}
			if vaapiTestParam.testNamePrefix == "cros_codecs_" {
				param.HardwareDeps = "hwdep.SupportsCrosCodecs()"
			}
			params = append(params, param)
		}

		// Generates VAAPI tests from bugs files
		for _, bugID := range test_vectors.SortedStringKeys(test_vectors.H264FilesFromBugs) {
			if vaapiTestParam.testNamePrefix == "cros_codecs_" && bugID == "299320432" {
				// This vector is legitimately malformed. Our regular VA-API decoder is just more tolerant than cros-codecs.
				continue
			}
			files := fmt.Sprintf("[]string{\"%s\"}", test_vectors.H264FilesFromBugs[bugID])
			param := paramData{
				Name:               fmt.Sprintf("%svaapi_h264_files_from_bugs_%s", vaapiTestParam.testNamePrefix, bugID),
				Decoder:            vaapiTestParam.binaryName,
				DecoderArgsBuilder: genDecoderArgsBuilder(vaapiTestParam.testNamePrefix, "h264"),
				Files:              files,
				Timeout:            defaultTimeout,
				SoftwareDeps:       []string{"vaapi", caps.HWDecodeH264},
				Metadata:           files,
				Attr:               []string{"graphics_video_h264", vaapiTestParam.frequency},
			}
			if vaapiTestParam.testNamePrefix == "cros_codecs_" {
				param.HardwareDeps = "hwdep.SupportsCrosCodecs()"
			}
			params = append(params, param)
		}
		for _, bugID := range test_vectors.SortedStringKeys(test_vectors.H2644kFilesFromBugs) {
			files := fmt.Sprintf("[]string{\"%s\"}", test_vectors.H2644kFilesFromBugs[bugID])
			param := paramData{
				Name:               fmt.Sprintf("%svaapi_h264_4k_files_from_bugs_%s", vaapiTestParam.testNamePrefix, bugID),
				Decoder:            vaapiTestParam.binaryName,
				DecoderArgsBuilder: genDecoderArgsBuilder(vaapiTestParam.testNamePrefix, "h264"),
				Files:              files,
				Timeout:            defaultTimeout,
				SoftwareDeps:       []string{"vaapi", caps.HWDecodeH264_4K},
				Metadata:           files,
				Attr:               []string{"graphics_video_h264", vaapiTestParam.frequency},
			}
			if vaapiTestParam.testNamePrefix == "cros_codecs_" {
				param.HardwareDeps = "hwdep.SupportsCrosCodecs()"
			}
			params = append(params, param)
		}
		for _, bugID := range test_vectors.SortedStringKeys(test_vectors.VP9FilesFromBugs) {
			files := fmt.Sprintf("[]string{\"%s\"}", test_vectors.VP9FilesFromBugs[bugID])
			param := paramData{
				Name:               fmt.Sprintf("%svaapi_vp9_files_from_bugs_%s", vaapiTestParam.testNamePrefix, bugID),
				Decoder:            vaapiTestParam.binaryName,
				DecoderArgsBuilder: genDecoderArgsBuilder(vaapiTestParam.testNamePrefix, "vp9"),
				Files:              files,
				Timeout:            defaultTimeout,
				SoftwareDeps:       []string{"vaapi", caps.HWDecodeVP9},
				Metadata:           files,
				Attr:               []string{"graphics_video_vp9", vaapiTestParam.frequency},
			}
			if vaapiTestParam.testNamePrefix == "cros_codecs_" {
				param.HardwareDeps = "hwdep.SupportsCrosCodecs()"
			}
			params = append(params, param)
		}
		for _, bugID := range test_vectors.SortedStringKeys(test_vectors.AV1FilesFromBugs) {
			files := fmt.Sprintf("[]string{\"%s\"}", test_vectors.AV1FilesFromBugs[bugID])
			param := paramData{
				Name:               fmt.Sprintf("%svaapi_av1_files_from_bugs_%s", vaapiTestParam.testNamePrefix, bugID),
				Decoder:            vaapiTestParam.binaryName,
				DecoderArgsBuilder: genDecoderArgsBuilder(vaapiTestParam.testNamePrefix, "av1"),
				Files:              files,
				Timeout:            defaultTimeout,
				SoftwareDeps:       []string{"vaapi", caps.HWDecodeAV1},
				Metadata:           files,
				Attr:               []string{"graphics_video_av1", vaapiTestParam.frequency},
			}
			if vaapiTestParam.testNamePrefix == "cros_codecs_" {
				param.HardwareDeps = "hwdep.SupportsCrosCodecs()"
			}
			params = append(params, param)
		}
		for _, bugID := range test_vectors.SortedStringKeys(test_vectors.H265FilesFromBugs) {
			files := fmt.Sprintf("[]string{\"%s\"}", test_vectors.H265FilesFromBugs[bugID])
			param := paramData{
				Name:               fmt.Sprintf("%svaapi_hevc_files_from_bugs_%s", vaapiTestParam.testNamePrefix, bugID),
				Decoder:            vaapiTestParam.binaryName,
				DecoderArgsBuilder: genDecoderArgsBuilder(vaapiTestParam.testNamePrefix, "hevc"),
				Files:              files,
				Timeout:            defaultTimeout,
				SoftwareDeps:       []string{"vaapi", caps.HWDecodeHEVC},
				Metadata:           files,
				Attr:               []string{"graphics_video_hevc", vaapiTestParam.frequency},
			}
			if vaapiTestParam.testNamePrefix == "cros_codecs_" {
				param.HardwareDeps = "hwdep.SupportsCrosCodecs()"
			}
			params = append(params, param)
		}
		for _, bugID := range test_vectors.SortedStringKeys(test_vectors.HEVCFilesFromBugs) {
			files := fmt.Sprintf("test_vectors.HEVCFilesFromBugs[\"%s\"]", bugID)
			param := paramData{
				Name:               fmt.Sprintf("%svaapi_hevc_files_from_bugs_%s", vaapiTestParam.testNamePrefix, bugID),
				Decoder:            vaapiTestParam.binaryName,
				DecoderArgsBuilder: genDecoderArgsBuilder(vaapiTestParam.testNamePrefix, "hevc"),
				Files:              files,
				Timeout:            time.Minute,
				SoftwareDeps:       []string{"vaapi", caps.HWDecodeHEVC},
				Metadata:           files,
				Attr:               []string{"graphics_video_hevc", vaapiTestParam.frequency},
			}
			if vaapiTestParam.testNamePrefix == "cros_codecs_" {
				param.HardwareDeps = "hwdep.SupportsCrosCodecs()"
			}
			params = append(params, param)
		}
	}

	params = append(params, paramData{
		Name:               "vaapi_vp9_0_svc",
		Decoder:            filepath.Join(chrome.BinTestDir, "decode_test"),
		DecoderArgsBuilder: "platform.VP9DecodeVAAPIargs",
		Files:              "test_vectors.VP9SVCFiles",
		Timeout:            defaultTimeout,
		SoftwareDeps:       []string{"vaapi", caps.HWDecodeVP9},
		Metadata:           "test_vectors.VP9SVCFiles",
		Attr:               []string{"graphics_video_vp9", "graphics_perbuild"},
	})

	// Generate V4L2 tests.
	for _, stateness := range []string{"Stateful", "Stateless"} {
		decoderExecutable := "v4l2_stateful_decoder"
		if stateness == "Stateless" {
			decoderExecutable = filepath.Join(chrome.BinTestDir, "v4l2_stateless_decoder")
		}
		decoderArgsBuilder := fmt.Sprintf("platform.V4L2%sDecodeArgs", stateness)
		commonHardwareDeps := []string{fmt.Sprintf("hwdep.SupportsV4L2%sVideoDecoding()", stateness)}

		// Generates V4L2 VP9 tests.
		for _, profile := range []string{"profile_0", "profile_2"} {
			for _, levelGroup := range []string{"group1", "group2", "group3", "group4", "level5_0", "level5_1"} {
				for _, cat := range []string{
					"buf", "frm_resize", "gf_dist", "odd_size", "sub8x8", "sub8x8_sf",
				} {

					// TODO(b/238211555) DRC is not supported with the current V4L2 stateless uAPI.
					if stateness == "Stateless" && (cat == "frm_resize" || cat == "sub8x8_sf") {
						continue
					}

					profileNum := string(profile[len(profile)-1])
					files := fmt.Sprintf("test_vectors.VP9WebmFiles[\"%s\"][\"%s\"][\"%s\"]", profile, levelGroup, cat)
					param := paramData{
						Name:               fmt.Sprintf("v4l2_%s_vp9_%s_%s_%s", strings.ToLower(stateness), profileNum, levelGroup, cat),
						Decoder:            decoderExecutable,
						DecoderArgsBuilder: decoderArgsBuilder,
						Files:              files,
						Timeout:            defaultTimeout,
						SoftwareDeps:       []string{"v4l2_codec"},
						Metadata:           files,
						Attr:               []string{"graphics_video_vp9"},
					}
					var ignoredSysLogs = []string{}
					if extension, ok := vp9GroupExtensions[levelGroup]; ok {
						param.Timeout = extension
					}
					if stateness == "Stateless" {
						param.Attr = append(param.Attr, "graphics_perbuild")
						// TODO(b/311261558) - when this resolves, we can remove the IgnoredSysLogs entry.
						ignoredSysLogs = append(ignoredSysLogs, "graphics.SysLogKernelSplats")
					} else {
						param.Attr = append(param.Attr, "graphics_weekly")
					}
					hardwareDeps := commonHardwareDeps

					if profile == "profile_2" {
						switch levelGroup {
						case "level5_0":
							param.SoftwareDeps = append(param.SoftwareDeps, caps.HWDecodeVP9_2_4K)
						case "level5_1":
							param.SoftwareDeps = append(param.SoftwareDeps, caps.HWDecodeVP9_2_4K60)
							hardwareDeps = append(hardwareDeps, "hwdep.MinMemory(7169)")
						default:
							param.SoftwareDeps = append(param.SoftwareDeps, caps.HWDecodeVP9_2)
						}
					} else {
						switch levelGroup {
						case "level5_0":
							param.SoftwareDeps = append(param.SoftwareDeps, caps.HWDecodeVP9_4K)
						case "level5_1":
							param.SoftwareDeps = append(param.SoftwareDeps, caps.HWDecodeVP9_4K60)
							hardwareDeps = append(hardwareDeps, "hwdep.MinMemory(7169)")
						default:
							param.SoftwareDeps = append(param.SoftwareDeps, caps.HWDecodeVP9)
						}
					}
					if stateness == "Stateful" && (profile != "profile_0" || levelGroup != "group1" || cat != "buf") {
						hardwareDeps = append(hardwareDeps, `hwdep.SkipGPUFamily("rogue")`)
					}

					param.IgnoredSysLogs = strings.Join(ignoredSysLogs, ", ")
					param.HardwareDeps = strings.Join(hardwareDeps, ", ")
					params = append(params, param)

					param.Name = "cros_codecs_" + param.Name
					param.HardwareDeps = param.HardwareDeps + ", " + "hwdep.SupportsCrosCodecs()"
					param.Decoder = ccdecPath
					param.DecoderArgsBuilder = genDecoderArgsBuilder(param.Name, "vp9")
					params = append(params, param)
				}
			}
		}

		// Generate V4L2 VP8 tests.
		for _, testGroup := range []string{"inter", "inter_multi_coeff", "inter_segment", "intra", "intra_multi_coeff", "intra_segment", "comprehensive"} {
			files := fmt.Sprintf("test_vectors.VP8Files[\"%s\"]", testGroup)

			hardwareDeps := commonHardwareDeps
			if stateness == "Stateful" {
				hardwareDeps = append(hardwareDeps, `hwdep.SkipGPUFamily("rogue")`)
			}

			param := paramData{
				Name:               fmt.Sprintf("v4l2_%s_vp8_%s", strings.ToLower(stateness), testGroup),
				Decoder:            decoderExecutable,
				DecoderArgsBuilder: decoderArgsBuilder,
				Files:              files,
				Timeout:            defaultTimeout,
				HardwareDeps:       strings.Join(hardwareDeps, ", "),
				SoftwareDeps:       []string{"v4l2_codec", caps.HWDecodeVP8},
				Metadata:           files,
				Attr:               []string{"graphics_video_vp8"},
			}
			var ignoredSysLogs = []string{}
			if stateness == "Stateless" {
				param.Attr = append(param.Attr, "graphics_perbuild")
				// TODO(b/311261558) - when this resolves, we can remove the IgnoredSysLogs entry.
				ignoredSysLogs = append(ignoredSysLogs, "graphics.SysLogKernelSplats")
			} else {
				param.Attr = append(param.Attr, "graphics_weekly")
			}

			param.IgnoredSysLogs = strings.Join(ignoredSysLogs, ", ")
			params = append(params, param)

			param.Name = "cros_codecs_" + param.Name
			param.HardwareDeps = param.HardwareDeps + ", " + "hwdep.SupportsCrosCodecs()"
			param.Decoder = ccdecPath
			param.DecoderArgsBuilder = genDecoderArgsBuilder(param.Name, "vp8")
			params = append(params, param)
		}

		// Generates V4L2 H264 tests.
		for _, group := range []string{"baseline", "main", "first_mb_in_slice"} {
			files := fmt.Sprintf("test_vectors.H264Files[\"%s\"]", group)

			hardwareDeps := commonHardwareDeps
			if stateness == "Stateful" {
				hardwareDeps = append(hardwareDeps, `hwdep.SkipGPUFamily("rogue")`)
			}

			// TODO(b/234752983): support first_mb_in_slice for Stateless decoder.
			if stateness == "Stateless" && group == "first_mb_in_slice" {
				continue
			}

			param := paramData{
				Name:               fmt.Sprintf("v4l2_%s_h264_%s", strings.ToLower(stateness), group),
				Decoder:            decoderExecutable,
				DecoderArgsBuilder: decoderArgsBuilder,
				Files:              files,
				Timeout:            defaultTimeout,
				SoftwareDeps:       []string{"v4l2_codec", caps.HWDecodeH264},
				HardwareDeps:       strings.Join(hardwareDeps, ", "),
				Metadata:           files,
				Attr:               []string{"graphics_video_h264"},
			}
			var ignoredSysLogs = []string{}
			if stateness == "Stateless" {
				param.Attr = append(param.Attr, "graphics_perbuild")
				// TODO(b/311261558) - when this resolves, we can remove the IgnoredSysLogs entry.
				ignoredSysLogs = append(ignoredSysLogs, "graphics.SysLogKernelSplats")
			} else {
				param.Attr = append(param.Attr, "graphics_weekly")
			}
			param.IgnoredSysLogs = strings.Join(ignoredSysLogs, ", ")
			params = append(params, param)

			param.Name = "cros_codecs_" + param.Name
			param.HardwareDeps = param.HardwareDeps + ", " + "hwdep.SupportsCrosCodecs()"
			param.Decoder = ccdecPath
			param.DecoderArgsBuilder = genDecoderArgsBuilder(param.Name, "h264")
			params = append(params, param)
		}

		// Generate V4L2 HEVC tests.
		for _, testGroup := range []string{"main_part_1", "main_part_2", "main_part_3", "main_part_4", "main_10", "mv_hevc"} {
			if stateness == "Stateful" && (testGroup == "main_10" || testGroup == "mv_hevc") {
				continue
			}
			files := fmt.Sprintf("test_vectors.HEVCFiles[\"%s\"]", testGroup)

			hardwareDeps := commonHardwareDeps
			if stateness == "Stateful" {
				hardwareDeps = append(hardwareDeps, `hwdep.SkipGPUFamily("rogue")`)
			}

			param := paramData{
				Name:               fmt.Sprintf("v4l2_%s_hevc_%s", strings.ToLower(stateness), testGroup),
				Decoder:            decoderExecutable,
				DecoderArgsBuilder: decoderArgsBuilder,
				Files:              files,
				Timeout:            defaultTimeout,
				HardwareDeps:       strings.Join(hardwareDeps, ", "),
				SoftwareDeps:       []string{"v4l2_codec", caps.HWDecodeHEVC},
				Metadata:           files,
				Attr:               []string{"graphics_video_hevc"},
			}
			var ignoredSysLogs = []string{}
			if stateness == "Stateless" {
				param.Attr = append(param.Attr, "graphics_perbuild")
				// TODO(b/311261558) - when this resolves, we can remove the IgnoredSysLogs entry.
				ignoredSysLogs = append(ignoredSysLogs, "graphics.SysLogKernelSplats")
			} else {
				param.Attr = append(param.Attr, "graphics_weekly")
			}
			param.IgnoredSysLogs = strings.Join(ignoredSysLogs, ", ")
			params = append(params, param)

			param.Name = "cros_codecs_" + param.Name
			param.HardwareDeps = param.HardwareDeps + ", " + "hwdep.SupportsCrosCodecs()"
			param.Decoder = ccdecPath
			param.DecoderArgsBuilder = genDecoderArgsBuilder(param.Name, "hevc")
			params = append(params, param)
		}

		// Generates V4L2 HEVC tests from bugs files.
		for _, bugID := range test_vectors.SortedStringKeys(test_vectors.HEVCFilesFromBugs) {
			if stateness == "Stateful" {
				continue
			}
			files := fmt.Sprintf("test_vectors.HEVCFilesFromBugs[\"%s\"]", bugID)

			hardwareDeps := commonHardwareDeps

			param := paramData{
				Name:               fmt.Sprintf("v4l2_%s_hevc_files_from_bugs_%s", strings.ToLower(stateness), bugID),
				Decoder:            decoderExecutable,
				DecoderArgsBuilder: decoderArgsBuilder,
				Files:              files,
				Timeout:            defaultTimeout,
				HardwareDeps:       strings.Join(hardwareDeps, ", "),
				SoftwareDeps:       []string{"v4l2_codec", caps.HWDecodeHEVC},
				Metadata:           files,
				Attr:               []string{"graphics_video_hevc"},
			}
			var ignoredSysLogs = []string{}
			if stateness == "Stateless" {
				param.Attr = append(param.Attr, "graphics_perbuild")
				// TODO(b/311261558) - when this resolves, we can remove the IgnoredSysLogs entry.
				ignoredSysLogs = append(ignoredSysLogs, "graphics.SysLogKernelSplats")
			} else {
				param.Attr = append(param.Attr, "graphics_weekly")
			}
			param.IgnoredSysLogs = strings.Join(ignoredSysLogs, ", ")
			params = append(params, param)

			param.Name = "cros_codecs_" + param.Name
			param.HardwareDeps = param.HardwareDeps + ", " + "hwdep.SupportsCrosCodecs()"
			param.Decoder = ccdecPath
			param.DecoderArgsBuilder = genDecoderArgsBuilder(param.Name, "hevc")
			params = append(params, param)
		}
	}

	params = append(params, paramData{
		Name:               "v4l2_q08c",
		Decoder:            "v4l2_stateful_decoder",
		DecoderArgsBuilder: "platform.V4L2StatefulDecodeArgsQ08C",
		Files:              "test_vectors.Q08cFiles",
		Timeout:            defaultTimeout,
		HardwareDeps:       "hwdep.SupportsV4L2StatefulVideoDecoding(), hwdep.GPUVendor(\"qualcomm\")",
		SoftwareDeps:       []string{"v4l2_codec", caps.HWDecodeH264, caps.HWDecodeVP8, caps.HWDecodeVP9},
		Metadata:           "test_vectors.Q08cFiles",
		Attr:               []string{"graphics_video_platformdecoding"},
	})

	// Generate V4L2 Stateless AV1 tests.
	// There are no V4L2 Stateful decoders that support AV1.  Once there are the AV1 tests can be moved into the general V4L2 generator loop.
	params = append(params, paramData{
		Name:               "v4l2_stateless_av1",
		Decoder:            filepath.Join(chrome.BinTestDir, "v4l2_stateless_decoder"),
		DecoderArgsBuilder: "platform.V4L2StatelessDecodeArgs",
		Files:              "test_vectors.AV1Files[\"8bit\"]",
		Timeout:            defaultTimeout,
		SoftwareDeps:       []string{"v4l2_codec", caps.HWDecodeAV1},
		// TODO(b/242075797): use HW capabilities.
		HardwareDeps: "hwdep.SupportsV4L2StatelessVideoDecoding()",
		Metadata:     "test_vectors.AV1Files[\"8bit\"]",
		Attr:         []string{"graphics_video_av1", "graphics_perbuild"},
	})

	params = append(params, paramData{
		Name:               "v4l2_stateless_av1_10bit",
		Decoder:            filepath.Join(chrome.BinTestDir, "v4l2_stateless_decoder"),
		DecoderArgsBuilder: "platform.V4L2StatelessDecodeArgs",
		Files:              "test_vectors.AV1Files[\"10bit\"]",
		Timeout:            defaultTimeout,
		SoftwareDeps:       []string{"v4l2_codec", caps.HWDecodeAV1},
		// TODO(b/242075797): use HW capabilities.
		HardwareDeps: "hwdep.SupportsV4L2StatelessVideoDecoding()",
		Metadata:     "test_vectors.AV1Files[\"10bit\"]",
		Attr:         []string{"graphics_video_av1", "graphics_perbuild"},
	})

	params = append(params, paramData{
		Name:               "v4l2_stateless_av1_10bit_quantizer",
		Decoder:            filepath.Join(chrome.BinTestDir, "v4l2_stateless_decoder"),
		DecoderArgsBuilder: "platform.V4L2StatelessDecodeArgs",
		Files:              "test_vectors.AV1Files[\"10bit_quantizer\"]",
		Timeout:            defaultTimeout,
		SoftwareDeps:       []string{"v4l2_codec", caps.HWDecodeAV1},
		// TODO(b/242075797): use HW capabilities.
		HardwareDeps: "hwdep.SupportsV4L2StatelessVideoDecoding()",
		Metadata:     "test_vectors.AV1Files[\"10bit_quantizer\"]",
		Attr:         []string{"graphics_video_av1", "graphics_perbuild"},
	})

	for _, cat := range []string{"quantizer", "size", "allintra", "cdfupdate", "motionvec"} {
		files := fmt.Sprintf("test_vectors.AV1Aom8bitFiles[\"%s\"]", cat)
		param := paramData{
			Name:               fmt.Sprintf("v4l2_stateless_av1_8bit_%s", cat),
			Decoder:            filepath.Join(chrome.BinTestDir, "v4l2_stateless_decoder"),
			DecoderArgsBuilder: "platform.V4L2StatelessDecodeArgs",
			Files:              files,
			// TODO(b/311261558) - when this resolves, we can remove the IgnoredSysLogs entry.
			IgnoredSysLogs: "graphics.SysLogKernelSplats",
			Timeout:        defaultTimeout,
			SoftwareDeps:   []string{"v4l2_codec", caps.HWDecodeAV1},
			// TODO(b/242075797): use HW capabilities.
			HardwareDeps: "hwdep.SupportsV4L2StatelessVideoDecoding()",
			Metadata:     files,
			Attr:         []string{"graphics_video_av1", "graphics_perbuild"},
		}

		params = append(params, param)
	}

	code := genparams.Template(t, `{{ range . }}{
		Name: {{ .Name | fmt }},
		Val:  platformDecodingParams{
			filenames: {{ .Files }},
			decoder: {{ .Decoder | fmt }},
			decoderArgsBuilder: {{ .DecoderArgsBuilder }},
			{{ if .IgnoredSysLogs }}
			ignoredSysLogs: []graphics.SysLogCategory{ {{ .IgnoredSysLogs }} },
			{{ end }}
		},
		Timeout: {{ .Timeout | fmt }},
		{{ if .HardwareDeps }}
		ExtraHardwareDeps: hwdep.D({{ .HardwareDeps }}),
		{{ end }}
		{{ if .SoftwareDeps }}
		ExtraSoftwareDeps: {{ .SoftwareDeps | fmt }},
		{{ end }}
		ExtraData: test_vectors.AppendJSONFiles({{ .Metadata }}),
		{{ if .Attr }}
		ExtraAttr: {{ .Attr | fmt }},
		{{ end }}
	},
	{{ end }}`, params)
	genparams.Ensure(t, "platform_decoding.go", code)
}
