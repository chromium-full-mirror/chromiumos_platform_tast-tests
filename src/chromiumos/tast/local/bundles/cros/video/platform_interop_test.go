// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"fmt"
	"testing"

	"chromiumos/tast/common/genparams"
	"chromiumos/tast/common/media/caps"
)

// NB: If modifying any of the files or test specifications, be sure to
// regenerate the test parameters by running the following in a chroot:
// TAST_GENERATE_UPDATE=1 ~/trunk/src/platform/tast/tools/go.sh test -count=1 chromiumos/tast/local/bundles/cros/video

type codecAPI string

const (
	software     codecAPI = "sw"
	vaapi        codecAPI = "vaapi"
	v4l2Stateful codecAPI = "v4l2sf"
	// TODO(b/251256531): Add other codecs and APIs.
)

func isHardwareAPI(api codecAPI) bool {
	return api == vaapi || api == v4l2Stateful
}

func isSoftwareAPI(api codecAPI) bool {
	return !isHardwareAPI(api)
}

func isMixedHardwareAPIs(encoder, decoder codecAPI) bool {
	return isHardwareAPI(encoder) && isHardwareAPI(decoder) && decoder != encoder
}

func getEncoderBinaryAndParams(encoder codecAPI, codec string) (binary, paramGenerator string) {
	switch encoder {
	case software:
		if codec == "vp8" || codec == "vp9" {
			return "vpxenc", "argsVpxenc"
		} else if codec == "h264" {
			return "openh264enc", "argsOpenh264enc"
		}
	case vaapi:
		if codec == "vp8" {
			return "vp8enc", "vp8argsVAAPI"
		} else if codec == "vp9" {
			return "vp9enc", "vp9argsVAAPI"
		} else if codec == "h264" {
			return "h264encode", "h264argsVAAPI"
		}
	case v4l2Stateful:
		return "v4l2_stateful_encoder", "argsV4L2"
	}
	return
}

// TODO(b/251256531): Move to a shared location and use it from platform_decoding*.go.
const decodeTestBinary = "/usr/local/libexec/chrome-binary-tests/decode_test"

func getDecoderBinaryAndParams(decoder codecAPI, codec string) (binary, paramGenerator string) {
	switch decoder {
	case software:
		if codec == "vp8" || codec == "vp9" {
			return "vpxdec", "vpxDecodeArgs"
		} else if codec == "h264" {
			return "openh264dec", "openh264DecodeArgs"
		}
	case vaapi:
		if codec == "vp8" {
			return decodeTestBinary, "vp8decodeVAAPIargs"
		} else if codec == "vp9" {
			return decodeTestBinary, "vp9decodeVAAPIargs"
		} else if codec == "h264" {
			return decodeTestBinary, "h264decodeVAAPIargs"
		}
	case v4l2Stateful:
		return "v4l2_stateful_decoder", "v4l2StatefulDecodeArgs"
	}
	return
}

func getSoftwareDeps(codec string, encoder, decoder codecAPI) []string {
	var deps []string
	if decoder == vaapi || encoder == vaapi {
		deps = append(deps, "vaapi")
	}
	if decoder == v4l2Stateful || encoder == v4l2Stateful {
		deps = append(deps, "v4l2_codec")
	}
	if isHardwareAPI(encoder) {
		if codec == "vp8" {
			deps = append(deps, caps.HWEncodeVP8)
		} else if codec == "vp9" {
			deps = append(deps, caps.HWEncodeVP9)
		} else if codec == "h264" {
			deps = append(deps, caps.HWEncodeH264)
		}
	}
	if isHardwareAPI(decoder) {
		if codec == "vp8" {
			deps = append(deps, caps.HWDecodeVP8)
		} else if codec == "vp9" {
			deps = append(deps, caps.HWDecodeVP9)
		} else if codec == "h264" {
			deps = append(deps, caps.HWDecodeH264)
		}
	}
	return deps
}

func getHardwareDeps(decoder codecAPI) string {
	if decoder == v4l2Stateful {
		return "hwdep.SupportsV4L2StatefulVideoDecoding()"
	}
	return ""
}

func TestPlatformInteropParamParams(t *testing.T) {
	type paramData struct {
		TestCaseName          string
		File                  string
		Resolution            string
		Fps                   int32
		EncoderCommand        string
		EncoderCommandBuilder string
		DecoderCommand        string
		DecoderArgsBuilder    string
		SoftwareDeps          []string
		HardwareDeps          string
		Data                  []string
	}
	var params []paramData

	type sourceFile struct {
		Name   string
		Width  int32
		Height int32
		Fps    int32
	}
	var sourceFiles = []sourceFile{{
		Name:   "gipsrestat-320x180.vp9.webm",
		Width:  320,
		Height: 180,
		Fps:    50,
	}}

	var codecs = []string{"vp8", "vp9", "h264"}
	var encoders = []codecAPI{software, vaapi, v4l2Stateful}
	var decoders = []codecAPI{software, vaapi, v4l2Stateful}
	for _, codec := range codecs {
		for _, encoder := range encoders {
			for _, decoder := range decoders {
				if isSoftwareAPI(encoder) && isSoftwareAPI(decoder) {
					// No need to verify interoperability of the SW reference implementation.
					continue
				}
				if isMixedHardwareAPIs(decoder, encoder) {
					// Skip mixing HW APIs.
					continue
				}

				for _, sourceFile := range sourceFiles {
					encoderBinary, encoderParamsGenerator := getEncoderBinaryAndParams(encoder, codec)
					decoderBinary, decoderParamsGenerator := getDecoderBinaryAndParams(decoder, codec)
					param := paramData{
						TestCaseName:          fmt.Sprintf("%s_%d_%s_to_%s", codec, sourceFile.Height, encoder, decoder),
						File:                  sourceFile.Name,
						Resolution:            fmt.Sprintf("coords.NewSize(%d, %d)", sourceFile.Width, sourceFile.Height),
						Fps:                   sourceFile.Fps,
						EncoderCommand:        encoderBinary,
						EncoderCommandBuilder: encoderParamsGenerator,
						DecoderCommand:        decoderBinary,
						DecoderArgsBuilder:    decoderParamsGenerator,
						SoftwareDeps:          getSoftwareDeps(codec, encoder, decoder),
						HardwareDeps:          getHardwareDeps(decoder),
						Data:                  []string{sourceFile.Name},
					}
					params = append(params, param)
				}
			}
		}
	}

	code := genparams.Template(t, `{{ range . }}{
		Name: {{ .TestCaseName | fmt }},
		Val:  platformInteropParam{
			filename:              {{ .File | fmt }},
			size:                  {{ .Resolution }},
			fps:                   {{ .Fps }},
			encoderCommand:        {{ .EncoderCommand | fmt }},
			encoderCommandBuilder: {{ .EncoderCommandBuilder }},
			decoderCommand:        {{ .DecoderCommand | fmt }},
			decoderArgsBuilder:    {{ .DecoderArgsBuilder }},
		},
		ExtraData: {{ .Data | fmt }},
		{{ if .SoftwareDeps }}
		ExtraSoftwareDeps: {{ .SoftwareDeps | fmt }},
		{{ end }}
		{{ if .HardwareDeps }}
		ExtraHardwareDeps: hwdep.D({{ .HardwareDeps }}),
		{{ end }}
	},
	{{ end }}`, params)
	genparams.Ensure(t, "platform_interop.go", code)
}
