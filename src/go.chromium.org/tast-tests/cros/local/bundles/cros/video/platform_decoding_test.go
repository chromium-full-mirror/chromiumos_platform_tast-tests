// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"go.chromium.org/tast-tests/cros/common/genparams"
	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/chrome"
)

const ffmpegMD5Path = "/usr/local/graphics/ffmpeg_md5sum"

// NB: If modifying any of the files or test specifications, be sure to
// regenerate the test parameters by running the following in a chroot:
// TAST_GENERATE_UPDATE=1 ~/chromiumos/src/platform/tast/tools/go.sh test -count=1 go.chromium.org/tast-tests/cros/local/bundles/cros/video

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

	// Generate VAAPI VP9 tests.
	for i, profile := range []string{"profile_0"} {
		for _, levelGroup := range []string{"group1", "group2", "group3", "group4", "level5_0", "level5_1"} {
			for _, cat := range []string{
				"buf", "frm_resize", "gf_dist", "odd_size", "sub8x8", "sub8x8_sf",
			} {
				files := fmt.Sprintf("vp9WebmFiles[\"%s\"][\"%s\"][\"%s\"]", profile, levelGroup, cat)
				param := paramData{
					Name:               fmt.Sprintf("vaapi_vp9_%d_%s_%s", i, levelGroup, cat),
					Decoder:            filepath.Join(chrome.BinTestDir, "decode_test"),
					DecoderArgsBuilder: "platform.VP9DecodeVAAPIargs",
					Files:              files,
					Timeout:            defaultTimeout,
					SoftwareDeps:       []string{"vaapi"},
					Metadata:           files,
					Attr:               []string{"graphics_video_vp9", "graphics_perbuild"},
				}
				if extension, ok := vp9GroupExtensions[levelGroup]; ok {
					param.Timeout = extension
				}

				var hardwareDeps []string

				// TODO(b/184683272): Reenable everywhere.
				if cat == "frm_resize" || cat == "sub8x8_sf" {
					hardwareDeps = append(hardwareDeps, "hwdep.SkipGPUFamily(\"picasso\")")
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

	params = append(params, paramData{
		Name:               fmt.Sprintf("vaapi_vp9_0_svc"),
		Decoder:            filepath.Join(chrome.BinTestDir, "decode_test"),
		DecoderArgsBuilder: "platform.VP9DecodeVAAPIargs",
		Files:              "vp9SVCFiles",
		Timeout:            defaultTimeout,
		SoftwareDeps:       []string{"vaapi", caps.HWDecodeVP9},
		Metadata:           "vp9SVCFiles",
		Attr:               []string{"graphics_video_vp9", "graphics_perbuild"},
	})

	// Generate VAAPI AV1 tests.
	params = append(params, paramData{
		Name:               "vaapi_av1",
		Decoder:            filepath.Join(chrome.BinTestDir, "decode_test"),
		DecoderArgsBuilder: "platform.AV1DecodeVAAPIargs",
		Files:              "av1Files[\"8bit\"]",
		Timeout:            defaultTimeout,
		// These SoftwareDeps do not include the 10 bit version of AV1.
		SoftwareDeps: []string{"vaapi", caps.HWDecodeAV1},
		Metadata:     "av1Files[\"8bit\"]",
		Attr:         []string{"graphics_video_av1", "graphics_perbuild"},
	})

	for _, cat := range []string{"quantizer", "size", "allintra", "cdfupdate", "motionvec"} {
		files := fmt.Sprintf("av1Aom8bitFiles[\"%s\"]", cat)
		param := paramData{
			Name:               fmt.Sprintf("vaapi_av1_8bit_%s", cat),
			Decoder:            filepath.Join(chrome.BinTestDir, "decode_test"),
			DecoderArgsBuilder: "platform.AV1DecodeVAAPIargs",
			Files:              files,
			Timeout:            defaultTimeout,
			// These SoftwareDeps do not include the 10 bit version of AV1.
			SoftwareDeps: []string{"vaapi", caps.HWDecodeAV1},
			Metadata:     files,
			Attr:         []string{"graphics_video_av1", "graphics_perbuild"},
		}

		params = append(params, param)
	}

	// Generate VAAPI HEVC tests.
	for _, testGroup := range []string{"main_part_1", "main_part_2", "main_part_3", "main_part_4"} {
		files := fmt.Sprintf("hevcFiles[\"%s\"]", testGroup)

		params = append(params, paramData{
			Name:               fmt.Sprintf("vaapi_hevc_%s", testGroup),
			Decoder:            filepath.Join(chrome.BinTestDir, "decode_test"),
			DecoderArgsBuilder: "platform.HEVCDecodeVAAPIargs",
			Files:              files,
			Timeout:            defaultTimeout,
			SoftwareDeps:       []string{"vaapi", caps.HWDecodeHEVC},
			Metadata:           files,
			Attr:               []string{"graphics_video_hevc", "graphics_perbuild"},
		})
	}

	// Generate VAAPI HEVC tests from bugs.
	for _, testGroup := range []string{"main"} {
		bugIDs := make([]string, 0, len(hevcFilesFromBugs[testGroup]))
		// Sort the keys so the order of output of the tests is deterministic.
		for k := range hevcFilesFromBugs[testGroup] {
			bugIDs = append(bugIDs, k)
		}
		sort.Strings(bugIDs)
		for _, bugID := range bugIDs {
			files := fmt.Sprintf("hevcFilesFromBugs[\"%s\"][\"%s\"]", testGroup, bugID)

			params = append(params, paramData{
				Name:               fmt.Sprintf("vaapi_hevc_%s_bug_%s", testGroup, bugID),
				Decoder:            filepath.Join(chrome.BinTestDir, "decode_test"),
				DecoderArgsBuilder: "platform.HEVCDecodeVAAPIargs",
				Files:              files,
				Timeout:            time.Minute,
				SoftwareDeps:       []string{"vaapi", caps.HWDecodeHEVC},
				Metadata:           files,
				Attr:               []string{"graphics_video_hevc", "graphics_perbuild"},
			})
		}
	}

	// Generate VAAPI VP8 tests.
	for _, testGroup := range []string{"inter", "inter_multi_coeff", "inter_segment", "intra", "intra_multi_coeff", "intra_segment", "comprehensive"} {
		files := fmt.Sprintf("vp8Files[\"%s\"]", testGroup)

		params = append(params, paramData{
			Name:               fmt.Sprintf("vaapi_vp8_%s", testGroup),
			Decoder:            filepath.Join(chrome.BinTestDir, "decode_test"),
			DecoderArgsBuilder: "platform.VP8DecodeVAAPIargs",
			Files:              files,
			Timeout:            defaultTimeout,
			SoftwareDeps:       []string{"vaapi", caps.HWDecodeVP8},
			Metadata:           files,
			Attr:               []string{"graphics_video_vp8", "graphics_perbuild"},
		})
	}

	// Generates VAAPI H264 tests.
	for _, group := range []string{"baseline", "main", "first_mb_in_slice"} {
		files := fmt.Sprintf("h264Files[\"%s\"]", group)

		param := paramData{
			Name:               fmt.Sprintf("vaapi_h264_%s", group),
			Decoder:            filepath.Join(chrome.BinTestDir, "decode_test"),
			DecoderArgsBuilder: "platform.H264DecodeVAAPIargs",
			Files:              files,
			Timeout:            defaultTimeout,
			SoftwareDeps:       []string{"vaapi", caps.HWDecodeH264},
			Metadata:           files,
			Attr:               []string{"graphics_video_h264", "graphics_perbuild"},
		}
		params = append(params, param)
	}

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
					files := fmt.Sprintf("vp9WebmFiles[\"%s\"][\"%s\"][\"%s\"]", profile, levelGroup, cat)
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
				}
			}
		}

		// Generate V4L2 VP8 tests.
		for _, testGroup := range []string{"inter", "inter_multi_coeff", "inter_segment", "intra", "intra_multi_coeff", "intra_segment", "comprehensive"} {
			files := fmt.Sprintf("vp8Files[\"%s\"]", testGroup)

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
		}

		// Generates V4L2 H264 tests.
		for _, group := range []string{"baseline", "main", "first_mb_in_slice"} {
			files := fmt.Sprintf("h264Files[\"%s\"]", group)

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
		}

		// Generate V4L2 HEVC tests.
		for _, testGroup := range []string{"main_part_1", "main_part_2", "main_part_3", "main_part_4", "main_10", "mv_hevc"} {
			if stateness == "Stateful" && (testGroup == "main_10" || testGroup == "mv_hevc") {
				continue
			}
			files := fmt.Sprintf("hevcFiles[\"%s\"]", testGroup)

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
		}
	}

	params = append(params, paramData{
		Name:               "v4l2_q08c",
		Decoder:            "v4l2_stateful_decoder",
		DecoderArgsBuilder: "platform.V4L2StatefulDecodeArgsQ08C",
		Files:              "q08cFiles",
		Timeout:            defaultTimeout,
		HardwareDeps:       "hwdep.SupportsV4L2StatefulVideoDecoding(), hwdep.GPUVendor(\"qualcomm\")",
		SoftwareDeps:       []string{"v4l2_codec", caps.HWDecodeH264, caps.HWDecodeVP8, caps.HWDecodeVP9},
		Metadata:           "q08cFiles",
		Attr:               []string{"graphics_video_platformdecoding"},
	})

	// Generate V4L2 Stateless AV1 tests.
	// There are no V4L2 Stateful decoders that support AV1.  Once there are the AV1 tests can be moved into the general V4L2 generator loop.
	params = append(params, paramData{
		Name:               "v4l2_stateless_av1",
		Decoder:            filepath.Join(chrome.BinTestDir, "v4l2_stateless_decoder"),
		DecoderArgsBuilder: "platform.V4L2StatelessDecodeArgs",
		Files:              "av1Files[\"8bit\"]",
		Timeout:            defaultTimeout,
		SoftwareDeps:       []string{"v4l2_codec", caps.HWDecodeAV1},
		// TODO(b/242075797): use HW capabilities.
		HardwareDeps: "hwdep.SupportsV4L2StatelessVideoDecoding()",
		Metadata:     "av1Files[\"8bit\"]",
		Attr:         []string{"graphics_video_av1", "graphics_perbuild"},
	})

	params = append(params, paramData{
		Name:               "v4l2_stateless_av1_10bit",
		Decoder:            filepath.Join(chrome.BinTestDir, "v4l2_stateless_decoder"),
		DecoderArgsBuilder: "platform.V4L2StatelessDecodeArgs",
		Files:              "av1Files[\"10bit\"]",
		Timeout:            defaultTimeout,
		SoftwareDeps:       []string{"v4l2_codec", caps.HWDecodeAV1},
		// TODO(b/242075797): use HW capabilities.
		HardwareDeps: "hwdep.SupportsV4L2StatelessVideoDecoding()",
		Metadata:     "av1Files[\"10bit\"]",
		Attr:         []string{"graphics_video_av1", "graphics_perbuild"},
	})

	params = append(params, paramData{
		Name:               "v4l2_stateless_av1_10bit_quantizer",
		Decoder:            filepath.Join(chrome.BinTestDir, "v4l2_stateless_decoder"),
		DecoderArgsBuilder: "platform.V4L2StatelessDecodeArgs",
		Files:              "av1Files[\"10bit_quantizer\"]",
		Timeout:            defaultTimeout,
		SoftwareDeps:       []string{"v4l2_codec", caps.HWDecodeAV1},
		// TODO(b/242075797): use HW capabilities.
		HardwareDeps: "hwdep.SupportsV4L2StatelessVideoDecoding()",
		Metadata:     "av1Files[\"10bit_quantizer\"]",
		Attr:         []string{"graphics_video_av1", "graphics_perbuild"},
	})

	for _, cat := range []string{"quantizer", "size", "allintra", "cdfupdate", "motionvec"} {
		files := fmt.Sprintf("av1Aom8bitFiles[\"%s\"]", cat)
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

	// Generate ffmpeg VAAPI VP9 tests.
	for i, profile := range []string{"profile_0"} {
		for _, levelGroup := range []string{"group1", "group2", "group3", "group4", "level5_0", "level5_1"} {
			for _, cat := range []string{
				"buf", "frm_resize", "gf_dist", "odd_size", "sub8x8", "sub8x8_sf",
			} {
				files := fmt.Sprintf("vp9WebmFiles[\"%s\"][\"%s\"][\"%s\"]", profile, levelGroup, cat)
				param := paramData{
					Name:               fmt.Sprintf("ffmpeg_vaapi_vp9_%d_%s_%s", i, levelGroup, cat),
					Decoder:            ffmpegMD5Path,
					DecoderArgsBuilder: "platform.FFMPEGMD5DecodeVAAPIArgs",
					Files:              files,
					Timeout:            defaultTimeout,
					SoftwareDeps:       []string{"vaapi"},
					Metadata:           files,
					Attr:               []string{"graphics_video_vp9", "graphics_nightly"},
				}
				if extension, ok := vp9GroupExtensions[levelGroup]; ok {
					param.Timeout = extension
				}

				var hardwareDeps []string

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

	// Generate ffmpeg VAAPI AV1 tests.
	params = append(params, paramData{
		Name:               "ffmpeg_vaapi_av1",
		Decoder:            filepath.Join(chrome.BinTestDir, "decode_test"),
		DecoderArgsBuilder: "platform.AV1DecodeVAAPIargs",
		Files:              "av1Files[\"8bit\"]",
		Timeout:            defaultTimeout,
		// These SoftwareDeps do not include the 10 bit version of AV1.
		SoftwareDeps: []string{"vaapi", caps.HWDecodeAV1},
		Metadata:     "av1Files[\"8bit\"]",
		Attr:         []string{"graphics_video_av1"},
	})

	for _, cat := range []string{"quantizer", "size", "allintra", "cdfupdate", "motionvec"} {
		files := fmt.Sprintf("av1Aom8bitFiles[\"%s\"]", cat)
		param := paramData{
			Name:               fmt.Sprintf("ffmpeg_vaapi_av1_8bit_%s", cat),
			Decoder:            filepath.Join(chrome.BinTestDir, "decode_test"),
			DecoderArgsBuilder: "platform.AV1DecodeVAAPIargs",
			Files:              files,
			Timeout:            defaultTimeout,
			// These SoftwareDeps do not include the 10 bit version of AV1.
			SoftwareDeps: []string{"vaapi", caps.HWDecodeAV1},
			Metadata:     files,
			Attr:         []string{"graphics_video_av1", "graphics_nightly"},
		}

		params = append(params, param)
	}

	// Generate ffmpeg VP8 tests.
	for _, testGroup := range []string{"inter", "inter_multi_coeff", "inter_segment", "intra", "intra_multi_coeff", "intra_segment", "comprehensive"} {
		files := fmt.Sprintf("vp8Files[\"%s\"]", testGroup)

		params = append(params, paramData{
			Name:               fmt.Sprintf("ffmpeg_vaapi_vp8_%s", testGroup),
			Decoder:            ffmpegMD5Path,
			DecoderArgsBuilder: "platform.FFMPEGMD5DecodeVAAPIArgs",
			Files:              files,
			Timeout:            defaultTimeout,
			SoftwareDeps:       []string{"vaapi", caps.HWDecodeVP8},
			Metadata:           files,
			Attr:               []string{"graphics_video_vp8", "graphics_nightly"},
		})
	}

	// Generate ffmpeg H264 tests.
	for _, group := range []string{"baseline", "main", "first_mb_in_slice"} {
		files := fmt.Sprintf("h264Files[\"%s\"]", group)

		param := paramData{
			Name:               fmt.Sprintf("ffmpeg_vaapi_h264_%s", group),
			Decoder:            ffmpegMD5Path,
			DecoderArgsBuilder: "platform.FFMPEGMD5DecodeVAAPIArgs",
			Files:              files,
			Timeout:            defaultTimeout,
			SoftwareDeps:       []string{"vaapi", caps.HWDecodeVP8},
			Metadata:           files,
			Attr:               []string{"graphics_video_h264", "graphics_nightly"},
		}
		params = append(params, param)
	}

	// Generate ffmpeg HEVC tests.
	for _, group := range []string{"main_part_1", "main_part_2", "main_part_3", "main_part_4"} {
		files := fmt.Sprintf("hevcFiles[\"%s\"]", group)

		param := paramData{
			Name:               fmt.Sprintf("ffmpeg_vaapi_hevc_%s", group),
			Decoder:            ffmpegMD5Path,
			DecoderArgsBuilder: "platform.FFMPEGMD5DecodeVAAPIArgs",
			Files:              files,
			Timeout:            defaultTimeout,
			SoftwareDeps:       []string{"vaapi", caps.HWDecodeHEVC},
			Metadata:           files,
			Attr:               []string{"graphics_video_hevc", "graphics_nightly"},
		}
		params = append(params, param)
	}

	// Generate ffmpeg HEVC tests from bugs.
	for _, testGroup := range []string{"main"} {
		bugIDs := make([]string, 0, len(hevcFilesFromBugs[testGroup]))
		// Sort the keys so the order of output of the tests is deterministic.
		for k := range hevcFilesFromBugs[testGroup] {
			bugIDs = append(bugIDs, k)
		}
		sort.Strings(bugIDs)
		for _, bugID := range bugIDs {
			files := fmt.Sprintf("hevcFilesFromBugs[\"%s\"][\"%s\"]", testGroup, bugID)

			params = append(params, paramData{
				Name:               fmt.Sprintf("ffmpeg_vaapi_hevc_%s_bug_%s", testGroup, bugID),
				Decoder:            ffmpegMD5Path,
				DecoderArgsBuilder: "platform.FFMPEGMD5DecodeVAAPIArgs",
				Files:              files,
				Timeout:            time.Minute,
				SoftwareDeps:       []string{"vaapi", caps.HWDecodeHEVC},
				Metadata:           files,
				Attr:               []string{"graphics_video_hevc", "graphics_nightly"},
			})
		}
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
		ExtraData: appendJSONFiles({{ .Metadata }}),
		{{ if .Attr }}
		ExtraAttr: {{ .Attr | fmt }},
		{{ end }}
	},
	{{ end }}`, params)
	genparams.Ensure(t, "platform_decoding.go", code)
}
