// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package webrtc

import (
	"fmt"
	"strings"
	"testing"

	"chromiumos/tast/local/bundles/cros/webrtc/peerconnection"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/graphics"
	"go.chromium.org/tast-tests/cros/common/genparams"
	"go.chromium.org/tast-tests/cros/common/media/caps"
)

// To regenerate the test parameters by running the following in a chroot:
// TAST_GENERATE_UPDATE=1 ~/trunk/src/platform/tast/tools/go.sh test -count=1 chromiumos/tast/local/bundles/cros/webrtc

// This matches peerconnection.RTCTestParams except that the members that are of
// enumerated types are made strings so that the generated Tast code retains the
// enumeration names.
type rtcTestParamsData struct {
	VerifyDecoderMode                     string
	VerifyEncoderMode                     string
	Profile                               string
	StreamWidth                           int
	StreamHeight                          int
	Svc                                   string
	Simulcasts                            int
	DisplayMediaType                      string
	VideoGridDimension                    int
	VideoGridFile                         string
	SimulcastHWEncs                       []bool
	BrowserType                           string
	VerifyOutOfProcessVideoEncodingIsUsed bool
}

type rtcPerfTestSourceData struct {
	Name string

	ParamData rtcTestParamsData

	SoftwareDeps []string
	HardwareDeps string
	Fixture      string
	Data         []string
}

type rtcPerfTestOOPOption int

const (
	noTestOption   rtcPerfTestOOPOption = iota
	outOfProcessVD                      // Out-of-process video decoding.
	outOfProcessVE                      // Out-of-process video encoding.
)

type captureSourceType struct {
	displayMediaType   peerconnection.DisplayMediaType
	zeroCopyTabCapture bool
}

var cameraCapture = captureSourceType{displayMediaType: "", zeroCopyTabCapture: false}

var k180p = graphics.Size{Width: 320, Height: 180}
var k270p = graphics.Size{Width: 480, Height: 270}
var k360p = graphics.Size{Width: 640, Height: 360}
var k720p = graphics.Size{Width: 1280, Height: 720}
var k1080p = graphics.Size{Width: 1920, Height: 1080}

func genFixture(verifyDecoderMode peerconnection.VerifyDecoderMode, verifyEncoderMode peerconnection.VerifyEncoderMode,
	useSvc bool, captureSource captureSourceType, browserType browser.Type, testOption rtcPerfTestOOPOption) string {
	if browserType == browser.TypeLacros {
		if verifyEncoderMode != peerconnection.VerifyHWEncoderUsed ||
			verifyDecoderMode != peerconnection.VerifyHWDecoderUsed ||
			useSvc || captureSource.displayMediaType != "" || testOption != noTestOption {
			panic("lacros testing does not currently support the requested options")
		}
		return "chromeVideoLacrosWithFakeWebcam"
	}
	if captureSource != cameraCapture {
		if verifyEncoderMode != peerconnection.VerifyHWEncoderUsed ||
			verifyDecoderMode != peerconnection.VerifyHWDecoderUsed ||
			useSvc || testOption != noTestOption {
			panic("display capture testing does not currently support the requested options")
		}
		var captureFixtureMap = map[peerconnection.DisplayMediaType]map[bool]string{
			peerconnection.CaptureMonitor: map[bool]string{false: "chromeScreenCapture", true: "chromeZeroCopyScreenCapture"},
			peerconnection.CaptureWindow:  map[bool]string{false: "chromeWindowCapture", true: "chromeZeroCopyWindowCapture"},
			peerconnection.CaptureTab:     map[bool]string{false: "chromeTabCapture", true: "chromeZeroCopyTabCapture"},
		}

		captureFixture, found := captureFixtureMap[captureSource.displayMediaType][captureSource.zeroCopyTabCapture]
		if !found {
			panic(fmt.Sprintf("unknown displayMediaType: %v", captureSource.displayMediaType))
		}
		return captureFixture
	}
	if useSvc {
		if verifyDecoderMode != peerconnection.VerifyHWDecoderUsed ||
			testOption != noTestOption {
			panic("svc testing does not currently support the requested options")
		}
		if verifyEncoderMode == peerconnection.VerifyHWEncoderUsed {
			return "chromeVideoWithFakeWebcamAndSVCEnabled"
		}
		return "chromeVideoWithFakeWebcamAndSVCEnabledAndSWEncoding"
	}
	if verifyEncoderMode == peerconnection.VerifyHWEncoderUsed {
		if verifyDecoderMode != peerconnection.VerifyHWDecoderUsed {
			panic("if encoder is hardware then the decoder must be also hardware currently")
		}

		if testOption == outOfProcessVD {
			return "chromeVideoOOPVDWithFakeWebcam"
		} else if testOption == outOfProcessVE {
			return "chromeVideoWithFakeWebcamAndOOPVE"
		}
		return "chromeVideoWithFakeWebcam"
	} else if verifyDecoderMode == peerconnection.VerifyHWDecoderUsed {
		if testOption != noTestOption {
			panic("peer connection testing does not currently support the requested options")
		}
		return "chromeVideoWithFakeWebcamAndSWEncoding"
	}
	return "chromeVideoWithFakeWebcamAndNoHwAcceleration"
}

func genTestName(codec string, resolution graphics.Size,
	verifyDecoderMode peerconnection.VerifyDecoderMode, verifyEncoderMode peerconnection.VerifyEncoderMode,
	svc string, simulcastHwEncs []bool, captureSource captureSourceType, browserType browser.Type, testOption rtcPerfTestOOPOption) string {
	testName := codec
	switch resolution {
	case k180p:
		testName += "_180p"
	case k270p:
		testName += "_270p"
	case k360p:
		testName += "_360p"
	case k720p:
		// Add no suffix because 720p is the default resolution.
	case k1080p:
		testName += "_1080p"
	default:
		panic(fmt.Sprintf("unknown resolution: %v", resolution))
	}

	hasSwEnc := false
	if len(simulcastHwEncs) == 0 {
		if verifyEncoderMode == peerconnection.VerifyHWEncoderUsed {
			testName += "_hw"
		} else {
			testName += "_sw"
			hasSwEnc = true
		}
		if svc != "" {
			testName += "_svc_" + strings.ToLower(svc)
		}
	} else {
		testName += "_simulcast"
		for i, hwEnc := range simulcastHwEncs {
			height := resolution.Height >> (len(simulcastHwEncs) - (i + 1))
			suffix := "sw"
			if hwEnc {
				suffix = "hw"
			} else {
				hasSwEnc = true
			}
			testName += fmt.Sprintf("_%d_%s", height, suffix)
		}
	}

	if captureSource != cameraCapture {
		switch captureSource.displayMediaType {
		case peerconnection.CaptureMonitor:
			testName += "_capture_monitor"
		case peerconnection.CaptureWindow:
			testName += "_capture_window"
		case peerconnection.CaptureTab:
			testName += "_capture_tab"
		default:
			panic(fmt.Sprintf("unknown DisplayMediaType: %v", captureSource.displayMediaType))
		}
		if captureSource.zeroCopyTabCapture {
			testName += "_zero_copy"
		}
	}
	if browserType == browser.TypeLacros {
		testName += "_lacros"
	}
	// outOfProcessVD is not handled here because the only the only current
	// OOP-VD variant is "hw_multi" and the test names are produced in-line on
	// TestRTCPeerConnectionPerfParams().
	if testOption == outOfProcessVE {
		testName += "_oopve"
	}
	// The sw_enc suffix is added to a test case where a hardware video decoder
	// is used but a software video encoder is used.
	if hasSwEnc && verifyDecoderMode == peerconnection.VerifyHWDecoderUsed {
		testName += "_enc"
	}
	return testName
}

func genSoftwareDeps(codec string,
	verifyDecoderMode peerconnection.VerifyDecoderMode,
	verifyEncoderMode peerconnection.VerifyEncoderMode,
	browserType browser.Type) []string {
	var softwareDeps []string
	if codec == "h264" {
		softwareDeps = append(softwareDeps, "proprietary_codecs")
	}

	// If the tests verifies that a hardware decoder is used, the test should only run
	// on devices that support hardware decoding. In those cases, we only care about
	// devices that support hardware encoding for the same codec (even if the test
	// ends up using software encoding), with one exception: vp9_1080p_sw_enc can run
	// on devices that support VP9 hardware decoding regardless of hardware encoder
	// support.
	if verifyDecoderMode == peerconnection.VerifyHWDecoderUsed {
		switch codec {
		case "h264":
			softwareDeps = append(softwareDeps, caps.HWEncodeH264, caps.HWDecodeH264)
		case "vp8":
			softwareDeps = append(softwareDeps, caps.HWEncodeVP8, caps.HWDecodeVP8)
		case "vp9":
			softwareDeps = append(softwareDeps, caps.HWEncodeVP9, caps.HWDecodeVP9)
		default:
			panic(fmt.Sprintf("unknown codec: %v", codec))
		}
	}
	if browserType == browser.TypeLacros {
		softwareDeps = append(softwareDeps, "lacros")
	}

	return softwareDeps
}

func genParamsData(codec string, resolution graphics.Size,
	verifyDecoderMode peerconnection.VerifyDecoderMode, verifyEncoderMode peerconnection.VerifyEncoderMode,
	svc string, simulcastHwEncs []bool,
	displayMediaType peerconnection.DisplayMediaType, browserType browser.Type) rtcTestParamsData {
	profile := strings.ToUpper(codec)
	var verifyDecoderModeStr, verifyEncoderModeStr, browserTypeStr, displayMediaTypeStr string
	switch verifyDecoderMode {
	case peerconnection.VerifyHWDecoderUsed:
		verifyDecoderModeStr = "peerconnection.VerifyHWDecoderUsed"
	case peerconnection.VerifySWDecoderUsed:
		verifyDecoderModeStr = "peerconnection.VerifySWDecoderUsed"
	case peerconnection.NoVerifyDecoderMode:
		verifyDecoderModeStr = "peerconnection.NoVerifyDecoderMode"
	default:
		panic(fmt.Sprintf("unknown verifyDecoderMode: %v", verifyDecoderMode))
	}
	switch verifyEncoderMode {
	case peerconnection.VerifyHWEncoderUsed:
		verifyEncoderModeStr = "peerconnection.VerifyHWEncoderUsed"
	case peerconnection.VerifySWEncoderUsed:
		verifyEncoderModeStr = "peerconnection.VerifySWEncoderUsed"
	case peerconnection.NoVerifyEncoderMode:
		verifyEncoderModeStr = "peerconnection.NoVerifyEncoderMode"
	default:
		panic(fmt.Sprintf("unknown verifyEncoderMode: %v", verifyEncoderMode))
	}
	switch browserType {
	case browser.TypeAsh:
		browserTypeStr = "browser.TypeAsh"
	case browser.TypeLacros:
		browserTypeStr = "browser.TypeLacros"
	default:
		panic(fmt.Sprintf("unknown browserType: %v", browserType))
	}
	switch displayMediaType {
	case peerconnection.CaptureMonitor:
		displayMediaTypeStr = "peerconnection.CaptureMonitor"
	case peerconnection.CaptureWindow:
		displayMediaTypeStr = "peerconnection.CaptureWindow"
	case peerconnection.CaptureTab:
		displayMediaTypeStr = "peerconnection.CaptureTab"
	}
	return rtcTestParamsData{
		VerifyDecoderMode:  verifyDecoderModeStr,
		VerifyEncoderMode:  verifyEncoderModeStr,
		Profile:            profile,
		StreamWidth:        resolution.Width,
		StreamHeight:       resolution.Height,
		VideoGridDimension: 1,
		Svc:                svc,
		Simulcasts:         len(simulcastHwEncs),
		SimulcastHWEncs:    simulcastHwEncs,
		DisplayMediaType:   displayMediaTypeStr,
		BrowserType:        browserTypeStr,
	}
}

func genRtcPerfTestSourceData(codec string, resolution graphics.Size,
	verifyDecoderMode peerconnection.VerifyDecoderMode, verifyEncoderMode peerconnection.VerifyEncoderMode,
	svc string, simulcastHWEncs []bool, captureSource captureSourceType,
	browserType browser.Type, testOption rtcPerfTestOOPOption) rtcPerfTestSourceData {
	return rtcPerfTestSourceData{
		Name: genTestName(codec, resolution, verifyDecoderMode, verifyEncoderMode,
			svc, simulcastHWEncs, captureSource, browserType, testOption),
		ParamData: genParamsData(codec, resolution, verifyDecoderMode, verifyEncoderMode,
			svc, simulcastHWEncs, captureSource.displayMediaType, browserType),
		SoftwareDeps: genSoftwareDeps(codec, verifyDecoderMode, verifyEncoderMode, browserType),
		Fixture:      genFixture(verifyDecoderMode, verifyEncoderMode, svc != "", captureSource, browserType, testOption),
	}
}

func TestRTCPeerConnectionPerfParams(t *testing.T) {
	verifyDecoderMode := map[bool]peerconnection.VerifyDecoderMode{
		false: peerconnection.VerifySWDecoderUsed, true: peerconnection.VerifyHWDecoderUsed,
	}
	verifyEncoderMode := map[bool]peerconnection.VerifyEncoderMode{
		false: peerconnection.VerifySWEncoderUsed, true: peerconnection.VerifyHWEncoderUsed,
	}

	var params []rtcPerfTestSourceData

	// Standard case.
	for _, codec := range []string{"h264", "vp8", "vp9", "av1"} {
		for _, hardware := range []bool{false, true} {
			if codec == "av1" && hardware {
				continue
			}
			param := genRtcPerfTestSourceData(codec, k720p,
				verifyDecoderMode[hardware], verifyEncoderMode[hardware],
				"", nil, cameraCapture, browser.TypeAsh, noTestOption)
			params = append(params, param)
		}
	}
	// Lacros.
	for _, codec := range []string{"h264", "vp8", "vp9"} {
		for _, resolution := range []graphics.Size{k720p, k360p} {
			param := genRtcPerfTestSourceData(codec, resolution,
				peerconnection.VerifyHWDecoderUsed, peerconnection.VerifyHWEncoderUsed,
				"", nil, cameraCapture, browser.TypeLacros, noTestOption)
			params = append(params, param)
		}
	}
	// Out-of-Process video encoding.
	for _, codec := range []string{"h264", "vp8", "vp9"} {
		param := genRtcPerfTestSourceData(codec, k720p,
			peerconnection.VerifyHWDecoderUsed, peerconnection.VerifyHWEncoderUsed,
			"", nil, cameraCapture, browser.TypeAsh, outOfProcessVE)
		params = append(params, param)
	}
	// VP9 1080p.
	for _, hardware := range []bool{false, true} {
		param := genRtcPerfTestSourceData("vp9", k1080p,
			verifyDecoderMode[hardware], verifyEncoderMode[hardware],
			"", nil, cameraCapture, browser.TypeAsh, noTestOption)
		params = append(params, param)
	}
	// vp9_1080p_sw_enc.
	param := genRtcPerfTestSourceData("vp9", k1080p,
		peerconnection.VerifyHWDecoderUsed, peerconnection.VerifySWEncoderUsed,
		"", nil, cameraCapture, browser.TypeAsh, noTestOption)
	// This is a special case in which HWEncodeVP9 is dropped even if we require to use a vp9 hardware decoder.
	param.SoftwareDeps = []string{caps.HWDecodeVP9}
	params = append(params, param)

	// VP8 display capture.
	for _, displayMediaType := range []peerconnection.DisplayMediaType{peerconnection.CaptureMonitor, peerconnection.CaptureWindow, peerconnection.CaptureTab} {
		for _, zeroCopyTabCapture := range []bool{false, true} {
			captureSource := captureSourceType{displayMediaType: displayMediaType, zeroCopyTabCapture: zeroCopyTabCapture}
			param := genRtcPerfTestSourceData("vp8", k720p,
				peerconnection.VerifyHWDecoderUsed, peerconnection.VerifyHWEncoderUsed,
				"", nil, captureSource, browser.TypeAsh, noTestOption)
			if displayMediaType == peerconnection.CaptureMonitor {
				param.HardwareDeps = "hwdep.InternalDisplay()"
			}
			params = append(params, param)
		}
	}
	// VP9 SVC.
	for _, svc := range []string{"L1T2", "L1T3", "L3T3_KEY"} {
		param := genRtcPerfTestSourceData("vp9", k720p,
			peerconnection.VerifyHWDecoderUsed, peerconnection.VerifyHWEncoderUsed,
			svc, nil, cameraCapture, browser.TypeAsh, noTestOption)
		params = append(params, param)
	}
	// VP8|VP9 hw multi.
	for _, disableVaapiLock := range []bool{false, true} {
		type multiTestParam struct {
			codec         string
			gridDimension int
			testOption    rtcPerfTestOOPOption
		}
		mtParams := []multiTestParam{{"vp8", 3, noTestOption}, {"vp8", 4, noTestOption}, {"vp9", 3, noTestOption}}
		// We test out-of-process video decoding only with the VA-API global lock enabled.
		if !disableVaapiLock {
			mtParams = append(mtParams, multiTestParam{"vp8", 3, outOfProcessVD})
		}
		for _, mp := range mtParams {
			var param rtcPerfTestSourceData
			param.Name = fmt.Sprintf("%s_hw_multi_vp9_%dx%d", mp.codec, mp.gridDimension, mp.gridDimension)
			param.ParamData = genParamsData(mp.codec, k720p,
				peerconnection.VerifyHWDecoderUsed, peerconnection.VerifyHWEncoderUsed,
				"", nil, "", browser.TypeAsh)
			param.ParamData.VideoGridDimension = mp.gridDimension
			param.ParamData.VideoGridFile = "tulip2-320x180.vp9.webm"
			param.Data = []string{param.ParamData.VideoGridFile}

			param.Fixture = "chromeVideoWithFakeWebcam"
			param.SoftwareDeps = []string{caps.HWDecodeVP9}
			if mp.codec == "vp8" {
				param.SoftwareDeps = append(param.SoftwareDeps, []string{caps.HWDecodeVP8, caps.HWEncodeVP8}...)
				if mp.gridDimension > 3 && !disableVaapiLock {
					param.HardwareDeps = "hwdep.SkipOnPlatform(\"trogdor\")"
				}
			} else {
				param.SoftwareDeps = append(param.SoftwareDeps, caps.HWEncodeVP9)
			}
			if disableVaapiLock {
				param.Name += "_global_vaapi_lock_disabled"
				param.SoftwareDeps = append(param.SoftwareDeps, "thread_safe_libva_backend")
				param.Fixture = "chromeVideoWithFakeWebcamAndGlobalVaapiLockDisabled"
			} else if mp.testOption == outOfProcessVD {
				param.Name += "_oopvd"
				param.Fixture = "chromeVideoOOPVDWithFakeWebcam"
			}

			params = append(params, param)
		}
	}

	// Encoder small resolution performance tests.
	for _, codec := range []string{"h264", "vp8", "vp9"} {
		for _, resolution := range []graphics.Size{k360p, k180p} {
			for _, hwEnc := range []bool{false, true} {
				param := genRtcPerfTestSourceData(codec, resolution,
					peerconnection.VerifyHWDecoderUsed,
					verifyEncoderMode[hwEnc],
					"", nil, cameraCapture, browser.TypeAsh, noTestOption)
				params = append(params, param)
			}
		}
	}
	// VP8 simulcast encoding tests.
	for _, resolution := range []graphics.Size{k720p, k360p} {
		var hwEncsPatterns [3][]bool
		switch resolution {
		case k360p:
			hwEncsPatterns = [3][]bool{
				[]bool{false, false},
				[]bool{false, true},
				[]bool{true, true},
			}
		case k720p:
			hwEncsPatterns = [3][]bool{
				[]bool{false, false, false},
				[]bool{false, true, true},
				[]bool{true, true, true},
			}
		}
		for _, hwEncs := range hwEncsPatterns {
			// minResOnlySwEnc is specified if a software encoder is used for
			// the smallest resolution and a hardware encoder is used for larger
			// resolution in the simulcast encodings.
			minResOnlySwEnc := hwEncs[0] == false
			for i := 1; i < len(hwEncs); i++ {
				if !hwEncs[i] {
					minResOnlySwEnc = false
					break
				}
			}
			// verifyEncoderMode is VerifyHWEncoderUsed HWEncoder if the encoder for the largest resolution is hardware one.
			verifyEncoderMode := peerconnection.VerifySWEncoderUsed
			if hwEncs[len(hwEncs)-1] {
				verifyEncoderMode = peerconnection.VerifyHWEncoderUsed
			}

			param := genRtcPerfTestSourceData("vp8", resolution,
				peerconnection.VerifyHWDecoderUsed, verifyEncoderMode,
				"", hwEncs, cameraCapture, browser.TypeAsh, noTestOption)
			if minResOnlySwEnc {
				param.SoftwareDeps = append(param.SoftwareDeps, "vaapi")
				param.Fixture = "chromeVideoWithFakeWebcamAndEnableVaapiVideoMinResolution"
			}
			params = append(params, param)
		}
	}
	// VP9 SVC (L2T3_KEY).
	for _, resolution := range []graphics.Size{k360p, k270p} {
		for _, hardware := range []bool{false, true} {
			param := genRtcPerfTestSourceData("vp9", resolution,
				peerconnection.VerifyHWDecoderUsed, verifyEncoderMode[hardware],
				"L2T3_KEY", nil, cameraCapture, browser.TypeAsh, noTestOption)
			param.HardwareDeps = "hwdep.SupportsVP9KSVCHWDecoding()"
			params = append(params, param)
		}
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
				{{ if (gt .ParamData.VideoGridDimension 1) }}
				VideoGridDimension: {{ .ParamData.VideoGridDimension }},
				{{ end }}
				{{ if .ParamData.VideoGridFile }}
				VideoGridFile: {{ .ParamData.VideoGridFile | fmt }},
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
			},
			{{ if .HardwareDeps }}
			ExtraHardwareDeps: hwdep.D({{ .HardwareDeps }}),
			{{ end }}
			{{ if .SoftwareDeps }}
			ExtraSoftwareDeps: {{ .SoftwareDeps | fmt }},
			{{ end }}
			{{ if .Data }}
			ExtraData: {{ .Data | fmt }},
			{{ end }}
			{{ if .Fixture }}
			Fixture: {{ .Fixture | fmt }},
			{{ end }}
		},
		{{ end }}`, params)

	genparams.Ensure(t, "rtc_peer_connection_perf.go", code)
}
