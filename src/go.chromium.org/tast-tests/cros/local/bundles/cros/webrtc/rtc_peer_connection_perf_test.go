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
	s3t3      streamType = "S3T3"
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
	case hwDec:
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
	if dec == hwDec {
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

	case l2t3key, l3t3key, s3t3:
		if codec != "vp9" {
			// Spatial layer encoding is supported only in vp9.
			return true
		}
	case simulcast:
		if codec != "vp8" && codec != "vp9" {
			// Simulcast encoding is used in vp8 and vp9 only today.
			return true
		}
	}
	return false
}

func toFixture(enc encoderImpl, dec decoderImpl, stream streamType) string {
	switch enc {
	case swEnc:
		switch dec {
		case swDec:
			return "chromeVideoWithFakeWebcamAndNoHwAcceleration"
		case hwDec:
			return "chromeVideoWithFakeWebcamAndSWEncoding"
		}
	case hwEnc:
		switch dec {
		case swDec:
			panic("we don't test hardware encoding + software decoding")
		case hwDec:
			return "chromeVideoWithFakeWebcam"
		}
	case oopVE:
		if stream == s3t3 {
			panic("we don't test OOP-VE + S-mode encoding")
		}
		switch dec {
		case swDec:
			panic("we don't test OOP-VE + software decoding")
		case hwDec:
			return "chromeVideoWithFakeWebcamAndOOPVE"
		}
	}
	panic(fmt.Sprintf("unexpected pair, enc=%s, dec=%s", string(enc), string(dec)))
}

func TestRTCPeerConnectionPerfParams(t *testing.T) {
	var sourceDatas []rtcPerfTestSourceData
	for _, codec := range []string{"h264", "vp8", "vp9", "av1"} {
		for _, resolution := range []graphics.Size{k720p, k1080p} {
			for _, stream := range []streamType{vanilla, l1t3, l2t3key, l3t3key, s3t3, simulcast} {
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
						}
						var streamTypeStr string
						if stream != vanilla {
							streamTypeStr = "_" + strings.ToLower(string(stream))
							if stream == simulcast {
								paramData.Simulcasts = 3
								if codec == "vp8" {
									// L1T1 because we want to run vp8 encoder tests on ChromeOS ARM,
									// where the vp8 temporal layer encoding is not supported.
									paramData.Svc = "L1T1"
									streamTypeStr += "_l1t1"
									for i := 0; i < paramData.Simulcasts; i++ {
										height := resolution.Height >> (paramData.Simulcasts - 1 - i)
										// The software encoder is used for a video whose resolution is less than 360p.
										hwEncForSimulcast := enc == hwEnc && height >= 360
										paramData.SimulcastHWEncs = append(paramData.SimulcastHWEncs, hwEncForSimulcast)
									}
								} else if codec == "vp9" {
									// L1T3 is because Google Meet uses it for vp9 simulcast.
									// VP9 simulcast test runs only on ChromeOS intel, where vp9 temporal layer encoding
									// is supported, so specifying this is not problem.
									paramData.Svc = "L1T3"
									streamTypeStr += "_l1t3"
									// Since VA-API video encoder supports S-mode encoding, a single VP9 VA-API encoder performs the simulcast encoding.
									// SimulcastHWEncs is thus not specified.
								}
							} else {
								paramData.Svc = string(stream)
							}
						}

						// TODO(b/378401081): Remove once vaapi av1 temporal SVC encoding enabled by default.
						fixture := toFixture(enc, dec, stream)
						if codec == "av1" && stream == l1t3 && enc == hwEnc {
							fixture = "chromeVideoWithFakeWebcamAndVaapiAv1TemporalSVC"
						}

						sourceData := rtcPerfTestSourceData{
							Name:         fmt.Sprintf("%s_%dp%s_%s_%s", codec, resolution.Height, streamTypeStr, enc, dec),
							ParamData:    paramData,
							SoftwareDeps: softwareCodecsDeps(codec, enc, dec),
							Fixture:      fixture,
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
	for _, codec := range []string{"h264", "vp8"} {
		for _, captureSource := range []peerconnection.DisplayMediaType{
			peerconnection.CaptureTab,
			peerconnection.CaptureWindow,
			peerconnection.CaptureMonitor,
		} {
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

			// Test with a hardware video decoding and encoding.
			// TODO(b/267966835): Test with software encoding if it is useful.
			enc := hwEnc
			dec := hwDec
			paramData := rtcTestParamsData{
				VerifyDecoderMode: toVerifyDecoderMode(dec),
				VerifyEncoderMode: toVerifyEncoderMode(enc),
				StreamWidth:       k1080p.Width,
				StreamHeight:      k1080p.Height,
				DisplayMediaType:  displayMediaTypeStr,
			}

			var svc string
			if codec == "vp8" {
				paramData.Profile = "VP8"
				paramData.Svc = "L1T3"
				svc = "_l1t3"
			} else {
				paramData.Profile = "H264"
			}

			var captureFixtureMap = map[peerconnection.DisplayMediaType]string{
				peerconnection.CaptureMonitor: "chromeScreenCapture",
				peerconnection.CaptureWindow:  "chromeWindowCapture",
				peerconnection.CaptureTab:     "chromeTabCapture",
			}
			sourceData := rtcPerfTestSourceData{
				Name:         fmt.Sprintf("%s_1080p_%s%s_hw_enc_hw_dec", codec, captureStr, svc),
				ParamData:    paramData,
				SoftwareDeps: softwareCodecsDeps(codec, enc, dec),
				Fixture:      captureFixtureMap[captureSource],
			}
			if captureSource == peerconnection.CaptureMonitor {
				sourceData.HardwareDeps = "hwdep.InternalDisplay(), hwdep.NoExternalDisplay()"
			}
			sourceDatas = append(sourceDatas, sourceData)
		}
	}

	// OOP-VE test cases.
	for _, codec := range []string{"h264", "vp8", "vp9", "av1"} {
		enc := oopVE
		dec := hwDec
		paramData := rtcTestParamsData{
			VerifyDecoderMode:                     toVerifyDecoderMode(dec),
			VerifyEncoderMode:                     toVerifyEncoderMode(enc),
			Profile:                               strings.ToUpper(codec),
			StreamWidth:                           k720p.Width,
			StreamHeight:                          k720p.Height,
			VerifyOutOfProcessVideoEncodingIsUsed: true,
		}
		sourceData := rtcPerfTestSourceData{
			Name:         fmt.Sprintf("%s_720p_%s_%s", codec, enc, dec),
			ParamData:    paramData,
			SoftwareDeps: softwareCodecsDeps(codec, enc, dec),
			Fixture:      toFixture(enc, dec, vanilla),
		}
		sourceDatas = append(sourceDatas, sourceData)
	}

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
