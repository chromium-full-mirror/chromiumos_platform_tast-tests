// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package webrtc

import (
	"fmt"
	"strings"
	"testing"

	"go.chromium.org/tast-tests/cros/common/genparams"
	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/webrtc/peerconnection"
	"go.chromium.org/tast-tests/cros/local/graphics"
)

// To regenerate the test parameters by running the following in a chroot:
// TAST_GENERATE_UPDATE=1 ~/chromiumos/src/platform/tast/tools/go.sh test -count=1 go.chromium.org/tast-tests/cros/local/bundles/cros/webrtc

// This matches peerconnection.RTCTestParams except for a couple of things:
//
//   - The members that are of enumerated types are made strings so that the
//     generated Tast code retains the enumeration names.
//
//   - RTCTestParams.VideoGridDimension and RTCTestParams.VideoGridFile are not
//     here because we don't have test cases that use it.
//     TODO(hiroh): remove those fields in RTCTestParams.
type rtcTestParamsData struct {
	VerifyDecoderMode                     string
	VerifyEncoderMode                     string
	Profile                               string
	StreamWidth                           int
	StreamHeight                          int
	Svc                                   string
	Simulcasts                            int
	SimulcastHWEncs                       []bool
	DisplayMediaType                      string
	BrowserType                           string
	VerifyOutOfProcessVideoEncodingIsUsed bool
	TraceChromeEvents                     bool
}

type rtcPerfTestSourceData struct {
	Name         string
	ParamData    rtcTestParamsData
	SoftwareDeps []string
	HardwareDeps string
	Fixture      string
}

var k720p = graphics.Size{Width: 1280, Height: 720}
var k1080p = graphics.Size{Width: 1920, Height: 1080}

type streamType string

const (
	vanilla   streamType = "vanilla"
	l1t3      streamType = "L1T3"
	l2t3key   streamType = "L2T3_KEY"
	l3t3key   streamType = "L3T3_KEY"
	simulcast streamType = "simulcast"
)

type encoderImpl string
type decoderImpl string

const (
	swEnc encoderImpl = "sw_enc"
	hwEnc encoderImpl = "hw_enc"
	oopVE encoderImpl = "hw_oopve"

	swDec decoderImpl = "sw_dec"
	hwDec decoderImpl = "hw_dec"
	oopVD decoderImpl = "hw_oopvd"
)

func isHardwareEncoderImpl(enc encoderImpl) bool {
	switch enc {
	case swEnc:
		return false
	case hwEnc, oopVE:
		return true
	}
	panic(fmt.Sprintf("unknown encoder: %v", enc))
}

func isHardwareDecoderImpl(dec decoderImpl) bool {
	switch dec {
	case swDec:
		return false
	case hwDec, oopVD:
		return true
	}
	panic(fmt.Sprintf("unknown decoder: %v", dec))
}

func toVerifyEncoderMode(enc encoderImpl) string {
	if isHardwareEncoderImpl(enc) {
		return "peerconnection.VerifyHWEncoderUsed"
	}
	return "peerconnection.VerifySWEncoderUsed"
}

func toVerifyDecoderMode(dec decoderImpl) string {
	if isHardwareDecoderImpl(dec) {
		return "peerconnection.VerifyHWDecoderUsed"
	}
	return "peerconnection.VerifySWDecoderUsed"
}

func softwareCodecsDeps(codec string, enc encoderImpl, dec decoderImpl) []string {
	var deps []string
	if codec == "h264" {
		deps = append(deps, "proprietary_codecs")
	}
	if enc == hwEnc || enc == oopVE {
		switch codec {
		case "h264":
			deps = append(deps, caps.HWEncodeH264)
		case "vp8":
			deps = append(deps, caps.HWEncodeVP8)
		case "vp9":
			deps = append(deps, caps.HWEncodeVP9)
		case "av1":
			deps = append(deps, caps.HWEncodeAV1)
		}
	}
	if dec == hwDec || dec == oopVD {
		switch codec {
		case "h264":
			deps = append(deps, caps.HWDecodeH264)
		case "vp8":
			deps = append(deps, caps.HWDecodeVP8)
		case "vp9":
			deps = append(deps, caps.HWDecodeVP9)
		case "av1":
			deps = append(deps, caps.HWDecodeAV1)
		}
	}
	return deps
}

func skipTest(codec string, stream streamType, enc encoderImpl, dec decoderImpl) bool {
	if isHardwareEncoderImpl(enc) && !isHardwareDecoderImpl(dec) {
		// There is no device that has a hardware encoder but no hardware decoder for any codec.
		return true
	}

	switch stream {
	case l1t3:
		if codec == "h264" {
			// H264 temporal layer encoding is not supported for webrtc.
			return true
		}
		if codec == "av1" && enc != swEnc {
			// AV1 temporal layer encoding is not supported by a hardware encoder.
			return true
		}

	case l2t3key, l3t3key:
		if codec != "vp9" {
			// Spatial layer encoding is supported only in vp9.
			return true
		}
	case simulcast:
		if codec != "vp8" {
			// Simulcast encoding is used in vp8 only today.
			return true
		}
	}
	return false
}

func toFixture(enc encoderImpl, dec decoderImpl) string {
	switch enc {
	case swEnc:
		switch dec {
		case swDec:
			return "chromeVideoWithFakeWebcamAndNoHwAcceleration"
		case hwDec:
			return "chromeVideoWithFakeWebcamAndSWEncoding"
		case oopVD:
			panic("we don't test OOP-VD + software encoding")
		}
	case hwEnc:
		switch dec {
		case swDec:
			panic("we don't test hardware encoding + software decoding")
		case hwDec:
			return "chromeVideoWithFakeWebcam"
		case oopVD:
			return "chromeVideoOOPVDWithFakeWebcam"
		}
	case oopVE:
		switch dec {
		case swDec:
			panic("we don't test OOP-VE + software decoding")
		case hwDec:
			return "chromeVideoWithFakeWebcamAndOOPVE"
		case oopVD:
			return "chromeVideoWithFakeWebcamAndOOPVDAndOOPVE"
		}
	}
	panic(fmt.Sprintf("unexpected pair, enc=%s, dec=%s", string(enc), string(dec)))
}

func TestRTCPeerConnectionPerfParams(t *testing.T) {
	var sourceDatas []rtcPerfTestSourceData
	for _, codec := range []string{"h264", "vp8", "vp9", "av1"} {
		for _, resolution := range []graphics.Size{k720p, k1080p} {
			for _, stream := range []streamType{vanilla, l1t3, l2t3key, l3t3key, simulcast} {
				for _, enc := range []encoderImpl{swEnc, hwEnc} {
					for _, dec := range []decoderImpl{swDec, hwDec} {
						if skipTest(codec, stream, enc, dec) {
							continue
						}

						paramData := rtcTestParamsData{
							VerifyDecoderMode: toVerifyDecoderMode(dec),
							VerifyEncoderMode: toVerifyEncoderMode(enc),
							Profile:           strings.ToUpper(codec),
							StreamWidth:       resolution.Width,
							StreamHeight:      resolution.Height,
							BrowserType:       "browser.TypeAsh",
							TraceChromeEvents: isHardwareDecoderImpl(dec) || isHardwareEncoderImpl(enc),
						}
						var streamTypeStr string
						if stream != vanilla {
							streamTypeStr = "_" + strings.ToLower(string(stream))
							if stream == simulcast {
								paramData.Simulcasts = 3
								for i := 0; i < paramData.Simulcasts; i++ {
									height := resolution.Height >> (paramData.Simulcasts - 1 - i)
									// The software encoder is used for a video whose resolution is less than 360p.
									hwEncForSimulcast := enc == hwEnc && height >= 360
									paramData.SimulcastHWEncs = append(paramData.SimulcastHWEncs, hwEncForSimulcast)
								}
							} else {
								paramData.Svc = string(stream)
							}
						}
						sourceData := rtcPerfTestSourceData{
							Name:         fmt.Sprintf("%s_%dp%s_%s_%s", codec, resolution.Height, streamTypeStr, enc, dec),
							ParamData:    paramData,
							SoftwareDeps: softwareCodecsDeps(codec, enc, dec),
							Fixture:      toFixture(enc, dec),
						}
						if dec == hwDec &&
							(stream == l2t3key || stream == l3t3key) {
							sourceData.HardwareDeps = "hwdep.SupportsVP9KSVCHWDecoding()"
						}
						sourceDatas = append(sourceDatas, sourceData)
					}
				}
			}
		}
	}

	// Display capture test cases.
	for _, captureSource := range []peerconnection.DisplayMediaType{
		peerconnection.CaptureTab,
		peerconnection.CaptureWindow,
		peerconnection.CaptureMonitor,
	} {
		for _, zeroCopy := range []bool{false, true} {
			var captureStr, displayMediaTypeStr string
			switch captureSource {
			case peerconnection.CaptureMonitor:
				captureStr += "monitor"
				displayMediaTypeStr = "peerconnection.CaptureMonitor"
			case peerconnection.CaptureWindow:
				captureStr += "window"
				displayMediaTypeStr = "peerconnection.CaptureWindow"
			case peerconnection.CaptureTab:
				captureStr += "tab"
				displayMediaTypeStr = "peerconnection.CaptureTab"
			}
			if zeroCopy {
				captureStr += "_zero_copy"
			}
			// Test with a hardware video decoding and encoding.
			// TODO(b/267966835): Test with software encoding if it is useful.
			enc := hwEnc
			dec := hwDec
			paramData := rtcTestParamsData{
				VerifyDecoderMode: toVerifyDecoderMode(dec),
				VerifyEncoderMode: toVerifyEncoderMode(enc),
				Profile:           "VP8",
				StreamWidth:       k1080p.Width,
				StreamHeight:      k1080p.Height,
				Svc:               "L1T3",
				DisplayMediaType:  displayMediaTypeStr,
				BrowserType:       "browser.TypeAsh",
				TraceChromeEvents: true,
			}
			var captureFixtureMap = map[peerconnection.DisplayMediaType]map[bool]string{
				peerconnection.CaptureMonitor: map[bool]string{false: "chromeScreenCapture", true: "chromeZeroCopyScreenCapture"},
				peerconnection.CaptureWindow:  map[bool]string{false: "chromeWindowCapture", true: "chromeZeroCopyWindowCapture"},
				peerconnection.CaptureTab:     map[bool]string{false: "chromeTabCapture", true: "chromeZeroCopyTabCapture"},
			}
			sourceData := rtcPerfTestSourceData{
				Name:         fmt.Sprintf("vp8_1080p_%s_l1t3_hw_enc_hw_dec", captureStr),
				ParamData:    paramData,
				SoftwareDeps: softwareCodecsDeps("vp8", enc, dec),
				Fixture:      captureFixtureMap[captureSource][zeroCopy],
			}
			if captureSource == peerconnection.CaptureMonitor {
				sourceData.HardwareDeps = "hwdep.InternalDisplay()"
			}
			sourceDatas = append(sourceDatas, sourceData)
		}
	}

	// OOP-VD and OOP-VE test cases.
	for _, codec := range []string{"h264", "vp8", "vp9", "av1"} {
		for _, ed := range [][]interface{}{
			{oopVE, hwDec},
			{oopVE, oopVD},
			{hwEnc, oopVD},
			// {swEnc, oopVD}, there is no fixture for this.
		} {
			var enc encoderImpl = ed[0].(encoderImpl)
			var dec decoderImpl = ed[1].(decoderImpl)
			paramData := rtcTestParamsData{
				VerifyDecoderMode: toVerifyDecoderMode(dec),
				VerifyEncoderMode: toVerifyEncoderMode(enc),
				Profile:           strings.ToUpper(codec),
				StreamWidth:       k720p.Width,
				StreamHeight:      k720p.Height,
				BrowserType:       "browser.TypeAsh",
				TraceChromeEvents: false,
			}
			if enc == oopVE {
				paramData.VerifyOutOfProcessVideoEncodingIsUsed = true
			}
			sourceData := rtcPerfTestSourceData{
				Name:         fmt.Sprintf("%s_720p_%s_%s", codec, enc, dec),
				ParamData:    paramData,
				SoftwareDeps: softwareCodecsDeps(codec, enc, dec),
				Fixture:      toFixture(enc, dec),
			}
			sourceDatas = append(sourceDatas, sourceData)
		}
	}

	// LaCrOS test cases.
	for _, codec := range []string{"h264", "vp8", "vp9", "av1"} {
		enc := hwEnc
		dec := hwDec
		paramData := rtcTestParamsData{
			VerifyDecoderMode: toVerifyDecoderMode(dec),
			VerifyEncoderMode: toVerifyEncoderMode(enc),
			Profile:           strings.ToUpper(codec),
			StreamWidth:       k720p.Width,
			StreamHeight:      k720p.Height,
			BrowserType:       "browser.TypeLacros",
			TraceChromeEvents: false,
		}
		sourceData := rtcPerfTestSourceData{
			Name:         fmt.Sprintf("%s_720p_lacros_hw_enc_hw_dec", codec),
			ParamData:    paramData,
			SoftwareDeps: softwareCodecsDeps(codec, enc, dec),
			Fixture:      "chromeVideoLacrosWithFakeWebcam",
		}
		sourceDatas = append(sourceDatas, sourceData)
	}

	// Tab capture + OOP-VD test case.
	tabCaptureOOPVDParamData := rtcTestParamsData{
		VerifyDecoderMode: toVerifyDecoderMode(oopVD),
		VerifyEncoderMode: toVerifyEncoderMode(swEnc),
		Profile:           "VP8",
		StreamWidth:       k1080p.Width,
		StreamHeight:      k1080p.Height,
		Svc:               "L1T3",
		DisplayMediaType:  "peerconnection.CaptureTab",
		BrowserType:       "browser.TypeAsh",
		TraceChromeEvents: false,
	}
	tabCaptureOOPVDSourceData := rtcPerfTestSourceData{
		Name:         "vp8_1080p_tab_l1t3_sw_enc_hw_oopvd",
		ParamData:    tabCaptureOOPVDParamData,
		SoftwareDeps: softwareCodecsDeps("vp8", swEnc, oopVD),
		Fixture:      "chromeTabCaptureWithOOPVDAndSWEncoding",
	}
	sourceDatas = append(sourceDatas, tabCaptureOOPVDSourceData)

	// Tab capture + LaCrOS test case.
	tabCaptureLacrosParamData := rtcTestParamsData{
		VerifyDecoderMode: toVerifyDecoderMode(hwDec),
		VerifyEncoderMode: toVerifyEncoderMode(swEnc),
		Profile:           "VP8",
		StreamWidth:       k1080p.Width,
		StreamHeight:      k1080p.Height,
		Svc:               "L1T3",
		DisplayMediaType:  "peerconnection.CaptureTab",
		BrowserType:       "browser.TypeLacros",
		TraceChromeEvents: false,
	}
	tabCaptureLacrosSourceData := rtcPerfTestSourceData{
		Name:         "vp8_1080p_tab_l1t3_lacros_sw_enc_hw_dec",
		ParamData:    tabCaptureLacrosParamData,
		SoftwareDeps: softwareCodecsDeps("vp8", swEnc, hwDec),
		Fixture:      "chromeTabCaptureWithLacrosAndSWEncoding",
	}
	sourceDatas = append(sourceDatas, tabCaptureLacrosSourceData)

	// Vaapi lock disabled test cases.
	for _, codec := range []string{"h264", "vp8", "vp9", "av1"} {
		dec := hwDec
		// TODO(b/183515570): Test with software encoding if it is useful.
		enc := hwEnc
		paramData := rtcTestParamsData{
			VerifyDecoderMode: toVerifyDecoderMode(dec),
			VerifyEncoderMode: toVerifyEncoderMode(enc),
			Profile:           strings.ToUpper(codec),
			StreamWidth:       k720p.Width,
			StreamHeight:      k720p.Height,
			BrowserType:       "browser.TypeAsh",
			TraceChromeEvents: true,
		}
		swDeps := softwareCodecsDeps(codec, enc, dec)
		swDeps = append(swDeps, "thread_safe_libva_backend")
		sourceData := rtcPerfTestSourceData{
			Name:         fmt.Sprintf("%s_720p_%s_%s_global_vaapi_lock_disabled", codec, enc, dec),
			ParamData:    paramData,
			SoftwareDeps: swDeps,
			Fixture:      "chromeVideoWithFakeWebcamAndGlobalVaapiLockDisabled",
		}
		sourceDatas = append(sourceDatas, sourceData)
	}
	code := genparams.Template(t, `{{ range . }}{
			Name: {{ .Name | fmt }},
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: {{ .ParamData.VerifyDecoderMode }},
				VerifyEncoderMode: {{ .ParamData.VerifyEncoderMode }},
				Profile: {{ .ParamData.Profile | fmt }},
				StreamWidth: {{ .ParamData.StreamWidth }},
				StreamHeight: {{ .ParamData.StreamHeight }},
				{{ if .ParamData.Svc }}
				Svc: {{ .ParamData.Svc | fmt}},
				{{ end }}
				{{ if .ParamData.DisplayMediaType }}
				DisplayMediaType: {{ .ParamData.DisplayMediaType }},
				{{ end }}
				{{ if .ParamData.Simulcasts }}
				Simulcasts: {{ .ParamData.Simulcasts }},
				{{ end }}
				{{ if .ParamData.SimulcastHWEncs }}
				SimulcastHWEncs: {{ .ParamData.SimulcastHWEncs | fmt }},
				{{ end }}
				BrowserType: {{ .ParamData.BrowserType }},
				{{ if .ParamData.VerifyOutOfProcessVideoEncodingIsUsed }}
				VerifyOutOfProcessVideoEncodingIsUsed: {{ .ParamData.VerifyOutOfProcessVideoEncodingIsUsed }},
				{{ end }}
                TraceChromeEvents: {{ .ParamData.TraceChromeEvents }},
			},
			{{ if .HardwareDeps }}
			ExtraHardwareDeps: hwdep.D({{ .HardwareDeps }}),
			{{ end }}
			{{ if .SoftwareDeps }}
			ExtraSoftwareDeps: {{ .SoftwareDeps | fmt }},
			{{ end }}
			{{ if .Fixture }}
			Fixture: {{ .Fixture | fmt }},
			{{ end }}
		},
		{{ end }}`, sourceDatas)

	genparams.Ensure(t, "rtc_peer_connection_perf.go", code)
}
