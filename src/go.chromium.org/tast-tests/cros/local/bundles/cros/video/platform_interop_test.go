// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"fmt"
	"testing"

	"go.chromium.org/tast-tests/cros/common/genparams"
	"go.chromium.org/tast-tests/cros/common/media/caps"
)

// NB: If modifying any of the files or test specifications, be sure to
// regenerate the test parameters by running the following in a chroot:
// TAST_GENERATE_UPDATE=1 ~/chromiumos/src/platform/tast/tools/go.sh test -count=1 go.chromium.org/tast-tests/cros/local/bundles/cros/video

type codecAPI string

const (
	software      codecAPI = "sw"
	vaapi         codecAPI = "vaapi"
	v4l2Stateful  codecAPI = "v4l2sf"
	v4l2Stateless codecAPI = "v4l2sl"
	// TODO(b/251256531): Add other codecs and APIs.
)

func isHardwareAPI(api codecAPI) bool {
	return api == vaapi || api == v4l2Stateful || api == v4l2Stateless
}

func isSoftwareAPI(api codecAPI) bool {
	return !isHardwareAPI(api)
}

// isMixedHardwareAPIs returns whether encoder and decoder use different hardware APIs.
func isMixedHardwareAPIs(encoder, decoder codecAPI) bool {
	if !isHardwareAPI(encoder) || !isHardwareAPI(decoder) {
		return false
	}

	// Returns true if encoder is vaapi and decoder is v4l2sl or v4l2sf, or
	// encoder is v4l2l or v4l2sf and decoder is vaapi.
	isVaapiEncoder := encoder == vaapi
	isVaapiDecoder := decoder == vaapi
	return isVaapiEncoder != isVaapiDecoder
}

func getEncoderBinaryAndParams(encoder codecAPI, codec string) (binary, paramGenerator string) {
	switch encoder {
	case software:
		switch codec {
		case "vp8", "vp9":
			return "vpxenc", "platform.ArgsVpxenc"
		case "h264":
			return "openh264enc", "platform.ArgsOpenh264enc"
		case "av1":
			return "aomenc", "platform.ArgsAomenc"
		}
	case vaapi:
		switch codec {
		case "vp8":
			return "vp8enc", "platform.VP8ArgsVAAPI"
		case "vp9":
			return "vp9enc", "platform.VP9ArgsVAAPI"
		case "h264":
			return "h264encode", "platform.H264ArgsVAAPI"
		case "av1":
			return "av1encode", "platform.AV1ArgsVAAPI"
		}
	case v4l2Stateful:
		return "v4l2_stateful_encoder", "platform.ArgsV4L2"
	}
	return
}

// TODO(b/251256531): Move to a shared location and use it from platform_decoding*.go.
const decodeTestBinary = "/usr/local/libexec/chrome-binary-tests/decode_test"

func getDecoderBinaryAndParams(decoder codecAPI, codec string) (binary, paramGenerator string) {
	switch decoder {
	case software:
		switch codec {
		case "vp8", "vp9":
			return "vpxdec", "platform.VPxDecodeArgs"
		case "h264":
			return "openh264dec", "platform.Openh264DecodeArgs"
		case "av1":
			return "dav1d", "platform.Dav1dDecodeArgs"
		}
	case vaapi:
		switch codec {
		case "vp8":
			return decodeTestBinary, "platform.VP8DecodeVAAPIargs"
		case "vp9":
			return decodeTestBinary, "platform.VP9DecodeVAAPIargs"
		case "h264":
			return decodeTestBinary, "platform.H264DecodeVAAPIargs"
		case "av1":
			return decodeTestBinary, "platform.AV1DecodeVAAPIargs"
		}
	case v4l2Stateful:
		return "v4l2_stateful_decoder", "platform.V4L2StatefulDecodeArgs"
	case v4l2Stateless:
		return "/usr/local/libexec/chrome-binary-tests/v4l2_stateless_decoder", "platform.V4L2StatelessDecodeArgs"
	}
	return
}

func getReferenceSWDecoder(decoder codecAPI, codec string) string {
	if isSoftwareAPI(decoder) {
		return "nil"
	}
	switch codec {
	case "vp8", "vp9":
		return "genMD5VPX"
	case "h264":
		return "genMD5FFMPEG"
	case "av1":
		return "genMD5AOM"
	}
	return "nil"
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
		switch codec {
		case "vp8":
			deps = append(deps, caps.HWEncodeVP8)
		case "vp9":
			deps = append(deps, caps.HWEncodeVP9)
		case "h264":
			deps = append(deps, caps.HWEncodeH264)
		case "av1":
			deps = append(deps, caps.HWEncodeAV1)
		}
	}
	if isHardwareAPI(decoder) {
		switch codec {
		case "vp8":
			deps = append(deps, caps.HWDecodeVP8)
		case "vp9":
			deps = append(deps, caps.HWDecodeVP9)
		case "h264":
			deps = append(deps, caps.HWDecodeH264)
		case "av1":
			deps = append(deps, caps.HWDecodeAV1)
		}
	}
	return deps
}

func getHardwareDeps(decoder codecAPI) string {
	if decoder == v4l2Stateful {
		return `hwdep.SupportsV4L2StatefulVideoDecoding(), hwdep.SkipGPUFamily("rogue")`
	}
	if decoder == v4l2Stateless {
		return "hwdep.SupportsV4L2StatelessVideoDecoding()"
	}
	return ""
}

func capableDeviceExists(codec string, encoder, decoder codecAPI) bool {
	// V4L2 encoder API is stateful only.
	if encoder == v4l2Stateless {
		return false
	}
	if codec == "vp9" {
		// No ChromeOS device using V4L2 supports vp9 hardware encoding.
		if encoder == v4l2Stateful {
			return false
		}
	}
	if codec == "av1" {
		// It is V4L2 stateless API that is used for av1 hardware decoding on
		// existing ChromeOS ARM devices.
		if decoder == v4l2Stateful {
			return false
		}
		// No ChromeOS device using V4L2 supports av1 hardware encoding.
		if encoder == v4l2Stateful {
			return false
		}
	}

	return true
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
		ReferenceSWDecoder    string
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

	var codecs = []string{"vp8", "vp9", "h264", "av1"}
	var encoders = []codecAPI{software, vaapi, v4l2Stateful}
	var decoders = []codecAPI{software, vaapi, v4l2Stateful, v4l2Stateless}
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
				if !capableDeviceExists(codec, encoder, decoder) {
					// Skip a test that will never runs on any device.
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
						ReferenceSWDecoder:    getReferenceSWDecoder(decoder, codec),
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
			referenceSWDecoder:    {{ .ReferenceSWDecoder }},
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
