// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package webrtc

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/webrtc/peerconnection"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	defaultRTCStreamWidth  = 1280
	defaultRTCStreamHeight = 720
	screenRTCStreamWidth   = 1920
	screenRTCStreamHeight  = 1080
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RTCPeerConnection,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verifies that WebRTC RTCPeerConnection works, maybe verifying use of a hardware accelerator",
		Contacts: []string{
			"hiroh@chromium.org",
			"chromeos-gfx-video@google.com",
			"mcasas@chromium.org", // Test author.
		},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video
		SoftwareDeps: []string{"chrome"},
		Data:         append(peerconnection.DataFiles(), peerconnection.LoopbackFile),
		Attr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
		Params: []testing.Param{{
			Name: "h264_no_verify_hw",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.NoVerifyEncoderMode,
				Profile:           "H264",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{"proprietary_codecs"},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name: "vp8_no_verify_hw",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.NoVerifyEncoderMode,
				Profile:           "VP8",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				BrowserType:       browser.TypeAsh,
			},
			Fixture: "chromeVideoWithFakeWebcam",
		}, {
			Name: "vp8_simulcast_no_verify_hw",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.NoVerifyEncoderMode,
				Profile:           "VP8",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				Simulcasts:        3,
				BrowserType:       browser.TypeAsh,
			},
			Fixture: "chromeVideoWithFakeWebcam",
		}, {
			Name: "vp9_no_verify_hw",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.NoVerifyEncoderMode,
				Profile:           "VP9",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				BrowserType:       browser.TypeAsh,
			},
			Fixture: "chromeVideoWithFakeWebcam",
		}, {
			Name: "av1_no_verify_hw",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.NoVerifyEncoderMode,
				Profile:           "AV1",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				BrowserType:       browser.TypeAsh,
			},
			Fixture: "chromeVideoWithFakeWebcam",
		}, {
			Name: "h264_verify_hw_dec",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.VerifyHWDecoderUsed,
				VerifyEncoderMode: peerconnection.NoVerifyEncoderMode,
				Profile:           "H264",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name: "h264_verify_hw_dec_gtfo",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.VerifyHWDecoderUsed,
				VerifyEncoderMode: peerconnection.NoVerifyEncoderMode,
				Profile:           "H264",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideoGTFOWithFakeWebcam",
		}, {
			Name: "h264_verify_hw_dec_v4l2_flat_stateful",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.VerifyHWDecoderUsed,
				VerifyEncoderMode: peerconnection.NoVerifyEncoderMode,
				Profile:           "H264",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsV4L2StatefulVideoDecoding()), // E.g. MT8173 Hana and QC SC7180 Trogdor.
			Fixture:           "chromeVideoWithFakeWebcamAndV4L2FlatStatefulDecoder",
		}, {
			Name: "h264_verify_hw_dec_inpvd",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.VerifyHWDecoderUsed,
				VerifyEncoderMode: peerconnection.NoVerifyEncoderMode,
				Profile:           "H264",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideoINPVDWithFakeWebcam",
		}, {
			Name: "vp8_verify_hw_dec",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.VerifyHWDecoderUsed,
				VerifyEncoderMode: peerconnection.NoVerifyEncoderMode,
				Profile:           "VP8",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP8},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name: "vp8_verify_hw_dec_v4l2_flat_stateful",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.VerifyHWDecoderUsed,
				VerifyEncoderMode: peerconnection.NoVerifyEncoderMode,
				Profile:           "VP8",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP8},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsV4L2StatefulVideoDecoding()), // E.g. MT8173 Hana and QC SC7180 Trogdor.
			Fixture:           "chromeVideoWithFakeWebcamAndV4L2FlatStatefulDecoder",
		}, {
			Name: "vp8_verify_hw_dec_inpvd",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.VerifyHWDecoderUsed,
				VerifyEncoderMode: peerconnection.NoVerifyEncoderMode,
				Profile:           "VP8",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP8},
			Fixture:           "chromeVideoINPVDWithFakeWebcam",
		}, {
			// This is a 2 temporal layers test, via the (experimental) API.
			// See https://www.w3.org/TR/webrtc-svc/#scalabilitymodes for SVC identifiers.
			Name: "vp8_svc_l1t2_verify_hw_dec",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.VerifyHWDecoderUsed,
				VerifyEncoderMode: peerconnection.NoVerifyEncoderMode,
				Profile:           "VP8",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				Svc:               "L1T2",
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP8},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			// This is an encoding test of 3 temporal layers test, via the (experimental) API.
			// See https://www.w3.org/TR/webrtc-svc/#scalabilitymodes for SVC identifiers.
			Name: "vp8_svc_l1t3_verify_hw_dec",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.VerifyHWDecoderUsed,
				VerifyEncoderMode: peerconnection.NoVerifyEncoderMode,
				Profile:           "VP8",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				Svc:               "L1T3",
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP8},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name: "vp9_verify_hw_dec",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.VerifyHWDecoderUsed,
				VerifyEncoderMode: peerconnection.NoVerifyEncoderMode,
				Profile:           "VP9",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name: "vp9_verify_hw_dec_v4l2_flat_stateful",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.VerifyHWDecoderUsed,
				VerifyEncoderMode: peerconnection.NoVerifyEncoderMode,
				Profile:           "VP9",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsV4L2StatefulVideoDecoding()), // E.g. MT8173 Hana and QC SC7180 Trogdor.
			Fixture:           "chromeVideoWithFakeWebcamAndV4L2FlatStatefulDecoder",
		}, {
			Name: "vp9_verify_hw_dec_inpvd",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.VerifyHWDecoderUsed,
				VerifyEncoderMode: peerconnection.NoVerifyEncoderMode,
				Profile:           "VP9",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9},
			Fixture:           "chromeVideoINPVDWithFakeWebcam",
		}, {
			Name: "vp9_1080p_verify_hw_dec",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.VerifyHWDecoderUsed,
				VerifyEncoderMode: peerconnection.NoVerifyEncoderMode,
				Profile:           "VP9",
				StreamWidth:       1920,
				StreamHeight:      1080,
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			// This is a decoding test of 2 temporal layers test, via the (experimental) API.
			// See https://www.w3.org/TR/webrtc-svc/#scalabilitymodes for SVC identifiers.
			Name: "vp9_svc_l1t2_verify_hw_dec",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.VerifyHWDecoderUsed,
				VerifyEncoderMode: peerconnection.NoVerifyEncoderMode,
				Profile:           "VP9",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				Svc:               "L1T2",
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			// This is a decoding test of 3 temporal layers test, via the (experimental) API.
			// See https://www.w3.org/TR/webrtc-svc/#scalabilitymodes for SVC identifiers.
			Name: "vp9_svc_l1t3_verify_hw_dec",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.VerifyHWDecoderUsed,
				VerifyEncoderMode: peerconnection.NoVerifyEncoderMode,
				Profile:           "VP9",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				Svc:               "L1T3",
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			// This is a decoding test of 3 spatial layers, 3 temporal layers (each) k-SVC test, via the (experimental) API.
			// See https://www.w3.org/TR/webrtc-svc/#scalabilitymodes for SVC identifiers.
			Name: "vp9_svc_l3t3_key_verify_hw_dec",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.VerifyHWDecoderUsed,
				VerifyEncoderMode: peerconnection.NoVerifyEncoderMode,
				Profile:           "VP9",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				Svc:               "L3T3_KEY",
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsVP9KSVCHWDecoding()),
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			// This is a decoding test of 3 spatial layers, 3 temporal layers (each) k-SVC test, via the (experimental) API.
			// See https://www.w3.org/TR/webrtc-svc/#scalabilitymodes for SVC identifiers.
			Name: "vp9_svc_l3t3_key_verify_hw_dec_gtfo",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.VerifyHWDecoderUsed,
				VerifyEncoderMode: peerconnection.NoVerifyEncoderMode,
				Profile:           "VP9",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				Svc:               "L3T3_KEY",
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsVP9KSVCHWDecoding()),
			Fixture:           "chromeVideoGTFOWithFakeWebcam",
		}, {
			// This is a decoding test of 3 spatial layers, 3 temporal layers (each) S-mode test, via the (experimental) API.
			// See https://www.w3.org/TR/webrtc-svc/#scalabilitymodes for SVC identifiers.
			Name: "vp9_svc_s3t3_verify_hw_dec",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.VerifyHWDecoderUsed,
				VerifyEncoderMode: peerconnection.NoVerifyEncoderMode,
				Profile:           "VP9",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				Svc:               "S3T3",
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9},
			Fixture:           "chromeVideoWithFakeWebcamAndHWSModeEncoding",
		}, {
			Name: "av1_verify_hw_dec",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.VerifyHWDecoderUsed,
				VerifyEncoderMode: peerconnection.NoVerifyEncoderMode,
				Profile:           "AV1",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWDecodeAV1},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			// This is a decoding test of 3 temporal layers test, via the (experimental) API.
			// See https://www.w3.org/TR/webrtc-svc/#scalabilitymodes for SVC identifiers.
			Name: "av1_svc_l1t3_verify_hw_dec",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.VerifyHWDecoderUsed,
				VerifyEncoderMode: peerconnection.NoVerifyEncoderMode,
				Profile:           "AV1",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				Svc:               "L1T3",
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWDecodeAV1},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name: "h264_verify_hw_enc",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.VerifyHWEncoderUsed,
				Profile:           "H264",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWEncodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name: "h264_capture_monitor_verify_hw_enc",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.VerifyHWEncoderUsed,
				Profile:           "H264",
				StreamWidth:       screenRTCStreamWidth,
				StreamHeight:      screenRTCStreamHeight,
				DisplayMediaType:  peerconnection.CaptureMonitor,
				BrowserType:       browser.TypeAsh,
			},
			ExtraHardwareDeps: hwdep.D(hwdep.InternalDisplay(), hwdep.NoExternalDisplay()),
			ExtraSoftwareDeps: []string{caps.HWEncodeH264, "proprietary_codecs"},
			Fixture:           "chromeScreenCapture",
		}, {
			Name: "h264_capture_window_verify_hw_enc",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.VerifyHWEncoderUsed,
				Profile:           "H264",
				StreamWidth:       screenRTCStreamWidth,
				StreamHeight:      screenRTCStreamHeight,
				DisplayMediaType:  peerconnection.CaptureWindow,
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWEncodeH264, "proprietary_codecs"},
			Fixture:           "chromeWindowCapture",
		}, {
			Name: "h264_capture_tab_verify_hw_enc",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.VerifyHWEncoderUsed,
				Profile:           "H264",
				StreamWidth:       screenRTCStreamWidth,
				StreamHeight:      screenRTCStreamHeight,
				DisplayMediaType:  peerconnection.CaptureTab,
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWEncodeH264, "proprietary_codecs"},
			Fixture:           "chromeTabCapture",
		}, {
			Name: "h264_capture_monitor_zero_copy_verify_hw_enc",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.VerifyHWEncoderUsed,
				Profile:           "H264",
				StreamWidth:       screenRTCStreamWidth,
				StreamHeight:      screenRTCStreamHeight,
				DisplayMediaType:  peerconnection.CaptureMonitor,
				BrowserType:       browser.TypeAsh,
			},
			ExtraHardwareDeps: hwdep.D(hwdep.InternalDisplay(), hwdep.NoExternalDisplay()),
			ExtraSoftwareDeps: []string{caps.HWEncodeH264, "proprietary_codecs"},
			Fixture:           "chromeZeroCopyScreenCapture",
		}, {
			Name: "h264_capture_window_zero_copy_verify_hw_enc",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.VerifyHWEncoderUsed,
				Profile:           "H264",
				StreamWidth:       screenRTCStreamWidth,
				StreamHeight:      screenRTCStreamHeight,
				DisplayMediaType:  peerconnection.CaptureWindow,
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWEncodeH264, "proprietary_codecs"},
			Fixture:           "chromeZeroCopyWindowCapture",
		}, {
			Name: "h264_capture_tab_zero_copy_verify_hw_enc",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.VerifyHWEncoderUsed,
				Profile:           "H264",
				StreamWidth:       screenRTCStreamWidth,
				StreamHeight:      screenRTCStreamHeight,
				DisplayMediaType:  peerconnection.CaptureTab,
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWEncodeH264, "proprietary_codecs"},
			Fixture:           "chromeZeroCopyTabCapture",
		}, {
			Name: "h264_lacros_verify_hw_enc",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.VerifyHWEncoderUsed,
				Profile:           "H264",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				BrowserType:       browser.TypeLacros,
			},
			ExtraSoftwareDeps: []string{caps.HWEncodeH264, "proprietary_codecs", "lacros"},
			Fixture:           "chromeVideoLacrosWithFakeWebcam",
		}, {
			Name: "h264_cam_verify_hw_enc",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.VerifyHWEncoderUsed,
				Profile:           "H264",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.BuiltinCamera, caps.HWEncodeH264, "proprietary_codecs"},
			Fixture:           "chromeCameraPerf",
		}, {
			Name: "h264_oopve_verify_hw_enc",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode:                     peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode:                     peerconnection.VerifyHWEncoderUsed,
				Profile:                               "H264",
				StreamWidth:                           defaultRTCStreamWidth,
				StreamHeight:                          defaultRTCStreamHeight,
				VerifyOutOfProcessVideoEncodingIsUsed: true,
				BrowserType:                           browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWEncodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideoWithFakeWebcamAndOOPVE",
		}, {
			Name: "vp8_verify_hw_enc",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.VerifyHWEncoderUsed,
				Profile:           "VP8",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name: "vp8_lacros_verify_hw_enc",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.VerifyHWEncoderUsed,
				Profile:           "VP8",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				BrowserType:       browser.TypeLacros,
			},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8, "lacros"},
			Fixture:           "chromeVideoLacrosWithFakeWebcam",
		}, {
			Name: "vp8_cam_verify_hw_enc",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.VerifyHWEncoderUsed,
				Profile:           "VP8",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.BuiltinCamera, caps.HWEncodeVP8},
			Fixture:           "chromeCameraPerf",
		}, {
			Name: "vp8_oopve_verify_hw_enc",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode:                     peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode:                     peerconnection.VerifyHWEncoderUsed,
				Profile:                               "VP8",
				StreamWidth:                           defaultRTCStreamWidth,
				StreamHeight:                          defaultRTCStreamHeight,
				VerifyOutOfProcessVideoEncodingIsUsed: true,
				BrowserType:                           browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8},
			Fixture:           "chromeVideoWithFakeWebcamAndOOPVE",
		}, {
			Name: "vp8_simulcast_verify_hw_enc",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.VerifyHWEncoderUsed,
				Profile:           "VP8",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				Simulcasts:        3,
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name: "vp8_capture_monitor_verify_hw_enc",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.VerifyHWEncoderUsed,
				Profile:           "VP8",
				StreamWidth:       screenRTCStreamWidth,
				StreamHeight:      screenRTCStreamHeight,
				DisplayMediaType:  peerconnection.CaptureMonitor,
				BrowserType:       browser.TypeAsh,
			},
			ExtraHardwareDeps: hwdep.D(hwdep.InternalDisplay(), hwdep.NoExternalDisplay()),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8},
			Fixture:           "chromeScreenCapture",
		}, {
			Name: "vp8_capture_window_verify_hw_enc",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.VerifyHWEncoderUsed,
				Profile:           "VP8",
				StreamWidth:       screenRTCStreamWidth,
				StreamHeight:      screenRTCStreamHeight,
				DisplayMediaType:  peerconnection.CaptureWindow,
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8},
			Fixture:           "chromeWindowCapture",
		}, {
			Name: "vp8_capture_tab_verify_hw_enc",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.VerifyHWEncoderUsed,
				Profile:           "VP8",
				StreamWidth:       screenRTCStreamWidth,
				StreamHeight:      screenRTCStreamHeight,
				DisplayMediaType:  peerconnection.CaptureTab,
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8},
			Fixture:           "chromeTabCapture",
		}, {
			// This is a 2 temporal layers test, via the (experimental) API.
			// See https://www.w3.org/TR/webrtc-svc/#scalabilitymodes for SVC identifiers.
			Name: "vp8_svc_l1t2_verify_hw_enc",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.VerifyHWEncoderUsed,
				Profile:           "VP8",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				Svc:               "L1T2",
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			// This is an encoding test of 3 temporal layers test, via the (experimental) API.
			// See https://www.w3.org/TR/webrtc-svc/#scalabilitymodes for SVC identifiers.
			Name: "vp8_svc_l1t3_verify_hw_enc",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.VerifyHWEncoderUsed,
				Profile:           "VP8",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				Svc:               "L1T3",
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name: "vp9_verify_hw_enc",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.VerifyHWEncoderUsed,
				Profile:           "VP9",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name: "vp9_lacros_verify_hw_enc",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.VerifyHWEncoderUsed,
				Profile:           "VP9",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				BrowserType:       browser.TypeLacros,
			},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			Fixture:           "chromeVideoLacrosWithFakeWebcam",
		}, {
			Name: "vp9_1080p_verify_hw_enc",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.VerifyHWEncoderUsed,
				Profile:           "VP9",
				StreamWidth:       1920,
				StreamHeight:      1080,
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name: "vp9_cam_verify_hw_enc",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.VerifyHWEncoderUsed,
				Profile:           "VP9",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.BuiltinCamera, caps.HWEncodeVP9},
			Fixture:           "chromeCameraPerf",
		}, {
			Name: "vp9_oopve_verify_hw_enc",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode:                     peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode:                     peerconnection.VerifyHWEncoderUsed,
				Profile:                               "VP9",
				StreamWidth:                           defaultRTCStreamWidth,
				StreamHeight:                          defaultRTCStreamHeight,
				VerifyOutOfProcessVideoEncodingIsUsed: true,
				BrowserType:                           browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			Fixture:           "chromeVideoWithFakeWebcamAndOOPVE",
		}, {
			// This is a 2 temporal layers test, via the (experimental) API.
			// See https://www.w3.org/TR/webrtc-svc/#scalabilitymodes for SVC identifiers.
			Name: "vp9_svc_l1t2_verify_hw_enc",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.VerifyHWEncoderUsed,
				Profile:           "VP9",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				Svc:               "L1T2",
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			// This is an encoding test of 3 temporal layers test, via the (experimental) API.
			// See https://www.w3.org/TR/webrtc-svc/#scalabilitymodes for SVC identifiers.
			Name: "vp9_svc_l1t3_verify_hw_enc",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.VerifyHWEncoderUsed,
				Profile:           "VP9",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				Svc:               "L1T3",
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			// This is an encoding test of 3 spatial layers, 3 temporal layers (each) k-SVC test, via the (experimental) API.
			// See https://www.w3.org/TR/webrtc-svc/#scalabilitymodes for SVC identifiers.
			Name: "vp9_svc_l3t3_key_verify_hw_enc",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.VerifyHWEncoderUsed,
				Profile:           "VP9",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				Svc:               "L3T3_KEY",
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			// This is an encoding test of 3 spatial layers, 3 temporal layers (each) S-mode test, via the (experimental) API.
			// See https://www.w3.org/TR/webrtc-svc/#scalabilitymodes for SVC identifiers.
			Name: "vp9_svc_s3t3_verify_hw_enc",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.VerifyHWEncoderUsed,
				Profile:           "VP9",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				Svc:               "S3T3",
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP9},
			Fixture:           "chromeVideoWithFakeWebcamAndHWSModeEncoding",
		}, {
			Name: "av1_verify_hw_enc",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.VerifyHWEncoderUsed,
				Profile:           "AV1",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWEncodeAV1},
			Fixture:           "chromeVideoWithFakeWebcam",
		}, {
			Name: "av1_capture_monitor_verify_hw_enc",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.VerifyHWEncoderUsed,
				Profile:           "AV1",
				StreamWidth:       screenRTCStreamWidth,
				StreamHeight:      screenRTCStreamHeight,
				DisplayMediaType:  peerconnection.CaptureMonitor,
				BrowserType:       browser.TypeAsh,
			},
			ExtraHardwareDeps: hwdep.D(hwdep.InternalDisplay(), hwdep.NoExternalDisplay()),
			ExtraSoftwareDeps: []string{caps.HWEncodeAV1, "proprietary_codecs"},
			Fixture:           "chromeScreenCapture",
		}, {
			Name: "av1_capture_window_verify_hw_enc",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.VerifyHWEncoderUsed,
				Profile:           "AV1",
				StreamWidth:       screenRTCStreamWidth,
				StreamHeight:      screenRTCStreamHeight,
				DisplayMediaType:  peerconnection.CaptureWindow,
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWEncodeAV1, "proprietary_codecs"},
			Fixture:           "chromeWindowCapture",
		}, {
			Name: "av1_capture_tab_verify_hw_enc",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.VerifyHWEncoderUsed,
				Profile:           "AV1",
				StreamWidth:       screenRTCStreamWidth,
				StreamHeight:      screenRTCStreamHeight,
				DisplayMediaType:  peerconnection.CaptureTab,
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWEncodeAV1, "proprietary_codecs"},
			Fixture:           "chromeTabCapture",
		}, {
			Name: "av1_capture_monitor_zero_copy_verify_hw_enc",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.VerifyHWEncoderUsed,
				Profile:           "AV1",
				StreamWidth:       screenRTCStreamWidth,
				StreamHeight:      screenRTCStreamHeight,
				DisplayMediaType:  peerconnection.CaptureMonitor,
				BrowserType:       browser.TypeAsh,
			},
			ExtraHardwareDeps: hwdep.D(hwdep.InternalDisplay(), hwdep.NoExternalDisplay()),
			ExtraSoftwareDeps: []string{caps.HWEncodeAV1, "proprietary_codecs"},
			Fixture:           "chromeZeroCopyScreenCapture",
		}, {
			Name: "av1_capture_window_zero_copy_verify_hw_enc",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.VerifyHWEncoderUsed,
				Profile:           "AV1",
				StreamWidth:       screenRTCStreamWidth,
				StreamHeight:      screenRTCStreamHeight,
				DisplayMediaType:  peerconnection.CaptureWindow,
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWEncodeAV1, "proprietary_codecs"},
			Fixture:           "chromeZeroCopyWindowCapture",
		}, {
			Name: "av1_capture_tab_zero_copy_verify_hw_enc",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.VerifyHWEncoderUsed,
				Profile:           "AV1",
				StreamWidth:       screenRTCStreamWidth,
				StreamHeight:      screenRTCStreamHeight,
				DisplayMediaType:  peerconnection.CaptureTab,
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWEncodeAV1, "proprietary_codecs"},
			Fixture:           "chromeZeroCopyTabCapture",
		}},
	})
}

// RTCPeerConnection verifies that a PeerConnection works correcttly and, if
// specified, verifies it uses accelerated encoding / decoding.
func RTCPeerConnection(ctx context.Context, s *testing.State) {
	params := s.Param().(peerconnection.RTCTestParams)

	cr, l, cs, err := lacros.Setup(ctx, s.FixtValue(), params.BrowserType)
	if err != nil {
		s.Fatal("Failed to initialize test: ", err)
	}
	defer lacros.CloseLacros(ctx, l)

	if err := peerconnection.RunRTCPeerConnection(
		ctx, cs, cr, s.DataFileSystem(), params); err != nil {
		s.Error("Failed to run RunRTCPeerConnection: ", err)
	}
}
