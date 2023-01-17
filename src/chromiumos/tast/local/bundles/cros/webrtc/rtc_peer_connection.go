// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package webrtc

import (
	"context"

	"chromiumos/tast/common/media/caps"
	"chromiumos/tast/local/bundles/cros/webrtc/peerconnection"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/lacros"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

const (
	defaultRTCStreamWidth  = 1280
	defaultRTCStreamHeight = 720
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RTCPeerConnection,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verifies that WebRTC RTCPeerConnection works, maybe verifying use of a hardware accelerator",
		Contacts: []string{
			"chromeos-gfx-video@google.com",
			"mcasas@chromium.org", // Test author.
			"hiroh@chromium.org",
		},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video
		SoftwareDeps: []string{"chrome"},
		Data:         append(peerconnection.DataFiles(), peerconnection.LoopbackFile),
		Attr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
		Params: []testing.Param{{
			Name: "h264",
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
			Name: "vp8",
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
			Name: "vp8_simulcast",
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
			Name: "vp9",
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
			Name: "h264_dec",
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
			Name: "h264_dec_alt",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.VerifyHWDecoderUsed,
				VerifyEncoderMode: peerconnection.NoVerifyEncoderMode,
				Profile:           "H264",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "video_decoder_legacy_supported", "proprietary_codecs"},
			Fixture:           "chromeVideoWithFakeWebcamAndAlternateVideoDecoder",
		}, {
			Name: "vp8_dec",
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
			Name: "vp8_dec_alt",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.VerifyHWDecoderUsed,
				VerifyEncoderMode: peerconnection.NoVerifyEncoderMode,
				Profile:           "VP8",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP8, "video_decoder_legacy_supported"},
			Fixture:           "chromeVideoWithFakeWebcamAndAlternateVideoDecoder",
		}, {
			Name: "vp9_dec",
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
			Name: "vp9_dec_alt",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.VerifyHWDecoderUsed,
				VerifyEncoderMode: peerconnection.NoVerifyEncoderMode,
				Profile:           "VP9",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9, "video_decoder_legacy_supported"},
			Fixture:           "chromeVideoWithFakeWebcamAndAlternateVideoDecoder",
		}, {
			Name: "vp9_dec_1080p",
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
			Name: "vp9_dec_svc_l1t2",
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
			Fixture:           "chromeVideoWithFakeWebcamAndSVCEnabled",
		}, {
			// This is a decoding test of 3 temporal layers test, via the (experimental) API.
			// See https://www.w3.org/TR/webrtc-svc/#scalabilitymodes for SVC identifiers.
			Name: "vp9_dec_svc_l1t3",
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
			Fixture:           "chromeVideoWithFakeWebcamAndSVCEnabled",
		}, {
			// This is a decoding test of 3 spatial layers, 3 temporal layers (each) k-SVC test, via the (experimental) API.
			// See https://www.w3.org/TR/webrtc-svc/#scalabilitymodes for SVC identifiers.
			Name: "vp9_dec_svc_l3t3_key",
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
			Fixture:           "chromeVideoWithFakeWebcamAndSVCEnabled",
		}, {
			// This is a decoding test of 3 spatial layers, 3 temporal layers (each) k-SVC test, via the (experimental) API.
			// See https://www.w3.org/TR/webrtc-svc/#scalabilitymodes for SVC identifiers.
			Name: "vp9_dec_svc_l3t3_key_oopvd",
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
			Fixture:           "chromeVideoOOPVDWithFakeWebcamAndSVCEnabled",
		}, {
			Name: "h264_enc",
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
			Name: "h264_enc_lacros",
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
			Name: "h264_enc_cam",
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
			Name: "h264_enc_oopve",
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
			Name: "vp8_enc",
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
			Name: "vp8_enc_lacros",
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
			Name: "vp8_enc_cam",
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
			Name: "vp8_enc_oopve",
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
			Name: "vp8_enc_simulcast",
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
			Name: "vp8_capture_monitor",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.VerifyHWEncoderUsed,
				Profile:           "VP8",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				DisplayMediaType:  peerconnection.CaptureMonitor,
				BrowserType:       browser.TypeAsh,
			},
			ExtraHardwareDeps: hwdep.D(hwdep.InternalDisplay()),
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8},
			Fixture:           "chromeScreenCapture",
		}, {
			Name: "vp8_capture_window",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.VerifyHWEncoderUsed,
				Profile:           "VP8",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				DisplayMediaType:  peerconnection.CaptureWindow,
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8},
			Fixture:           "chromeWindowCapture",
		}, {
			Name: "vp8_capture_tab",
			Val: peerconnection.RTCTestParams{
				VerifyDecoderMode: peerconnection.NoVerifyDecoderMode,
				VerifyEncoderMode: peerconnection.VerifyHWEncoderUsed,
				Profile:           "VP8",
				StreamWidth:       defaultRTCStreamWidth,
				StreamHeight:      defaultRTCStreamHeight,
				DisplayMediaType:  peerconnection.CaptureTab,
				BrowserType:       browser.TypeAsh,
			},
			ExtraSoftwareDeps: []string{caps.HWEncodeVP8},
			Fixture:           "chromeTabCapture",
		}, {
			// This is a 2 temporal layers test, via the (experimental) API.
			// See https://www.w3.org/TR/webrtc-svc/#scalabilitymodes for SVC identifiers.
			Name: "vp8_enc_svc_l1t2",
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
			Fixture:           "chromeVideoWithFakeWebcamAndSVCEnabledWithHWVp8TemporalLayerEncoding",
		}, {
			// This is an encoding test of 3 temporal layers test, via the (experimental) API.
			// See https://www.w3.org/TR/webrtc-svc/#scalabilitymodes for SVC identifiers.
			Name: "vp8_enc_svc_l1t3",
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
			Fixture:           "chromeVideoWithFakeWebcamAndSVCEnabledWithHWVp8TemporalLayerEncoding",
		}, {
			Name: "vp9_enc",
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
			Name: "vp9_enc_lacros",
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
			Name: "vp9_enc_1080p",
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
			Name: "vp9_enc_cam",
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
			Name: "vp9_enc_oopve",
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
			Name: "vp9_enc_svc_l1t2",
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
			Fixture:           "chromeVideoWithFakeWebcamAndSVCEnabled",
		}, {
			// This is an encoding test of 3 temporal layers test, via the (experimental) API.
			// See https://www.w3.org/TR/webrtc-svc/#scalabilitymodes for SVC identifiers.
			Name: "vp9_enc_svc_l1t3",
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
			Fixture:           "chromeVideoWithFakeWebcamAndSVCEnabled",
		}, {
			// This is an encoding test of 3 spatial layers, 3 temporal layers (each) k-SVC test, via the (experimental) API.
			// See https://www.w3.org/TR/webrtc-svc/#scalabilitymodes for SVC identifiers.
			Name: "vp9_enc_svc_l3t3_key",
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
			Fixture:           "chromeVideoWithFakeWebcamAndSVCEnabled",
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
