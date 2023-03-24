// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path"
	"time"

	"chromiumos/tast/common/media/caps"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/metrics"
	"chromiumos/tast/local/media/constants"
	"chromiumos/tast/local/media/histogram"
	"chromiumos/tast/testing"
)

type verifyHWAcceleratorMode int

const (
	// verifyLegacyVDAPathWasUsed is a mode that verifies that a hardware decoder backed by the legacy VideoDecodeAccelerator path was used.
	verifyLegacyVDAPathWasUsed verifyHWAcceleratorMode = iota
	// verifyMojoVDPathWasUsed is a mode that verifies that a hardware decoder backed by the the newer MojoVideoDecoder path was used.
	verifyMojoVDPathWasUsed
	// verifySWPathWasUsed is a mode that verifies that fallback to the software decoding path happens after trying to use the MojoVideoDecoder path.
	verifySWPathWasUsed
)

type pepperVideoDecodeTestParam struct {
	browserType  browser.Type
	verifyHWMode verifyHWAcceleratorMode
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         PepperVideoDecode,
		LacrosStatus: testing.LacrosVariantUnknown,
		Desc:         "Checks that simple video playback in Pepper (NaCl) is working",
		Contacts: []string{
			"chromeos-gfx-video@google.com",
			"pmolinalopez@chromium.org",
		},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video
		Data: []string{
			"pepper/video_decode/pnacl/Release/video_decode.nmf",
			"pepper/video_decode/pnacl/Release/video_decode.pexe",
			"pepper/video_decode/video_decode.html",
		},
		SoftwareDeps: []string{"chrome", "nacl"},
		Params: []testing.Param{{
			Name:              "h264_hw",
			Val:               pepperVideoDecodeTestParam{browserType: browser.TypeAsh, verifyHWMode: verifyLegacyVDAPathWasUsed},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "video_decoder_legacy_supported", "proprietary_codecs"},
			Fixture:           "chromeVideoNaCl",
		}, {
			Name:              "h264_hw_mojovd",
			Val:               pepperVideoDecodeTestParam{browserType: browser.TypeAsh, verifyHWMode: verifyMojoVDPathWasUsed},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideoNaClWithMojoVideoDecoder",
		}, {
			Name:              "h264_sw",
			Val:               pepperVideoDecodeTestParam{browserType: browser.TypeAsh, verifyHWMode: verifySWPathWasUsed},
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraSoftwareDeps: []string{"proprietary_codecs"},
			Fixture:           "chromeVideoNaClWithSWDecoding",
		}},
	})
}

func PepperVideoDecode(ctx context.Context, s *testing.State) {
	params := s.Param().(pepperVideoDecodeTestParam)

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	cr := s.FixtValue().(*chrome.Chrome)

	ctconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	hwBehaviourHistogramName := constants.MediaPepperVideoDecoderHardwareAccelerationBehavior
	initHistogram, err := metrics.GetHistogram(ctx, ctconn, hwBehaviourHistogramName)
	if err != nil {
		s.Fatal("Failed to get initial histogram: ", err)
	}

	url := path.Join(server.URL, "pepper/video_decode/video_decode.html")
	conn, err := cr.NewConn(ctx, url)
	if err != nil {
		s.Fatalf("Failed to open %v: %v", url, err)
	}
	defer conn.Close()

	// Check that the NaCl video decoding example loaded correctly.
	if err := conn.WaitForExprWithTimeout(ctx, "doneLoadingExample", 10*time.Second); err != nil {
		s.Fatal("The NaCl app did not load in time: ", err)
	}

	// Minimize and maximize browser window. Needed to trigger the video playback.
	// TODO(pmolinalopez): remove when crbug.com/1376105 is solved.
	w, err := ash.WaitForAnyWindowWithTitle(ctx, ctconn, "Pepper video decoder")
	if err != nil {
		s.Fatal("Failed to find the window that contains the NaCl app: ", err)
	}
	if err := ash.SetWindowStateAndWait(ctx, ctconn, w.ID, ash.WindowStateMinimized); err != nil {
		s.Fatal("Failed to minimize the window that contains the NaCl app: ", err)
	}
	if err := ash.SetWindowStateAndWait(ctx, ctconn, w.ID, ash.WindowStateMaximized); err != nil {
		s.Fatal("Failed to maximize the window that contains the NaCl app: ", err)
	}

	var hwBehaviourSucessValue int64
	switch params.verifyHWMode {
	case verifyMojoVDPathWasUsed:
		hwBehaviourSucessValue = int64(constants.MediaPepperVideoDecoderHardwareAccelerationBehaviorWithMojoVD)
	case verifyLegacyVDAPathWasUsed:
		hwBehaviourSucessValue = int64(constants.MediaPepperVideoDecoderHardwareAccelerationBehaviorWithoutMojoVD)
	case verifySWPathWasUsed:
		hwBehaviourSucessValue = int64(constants.MediaPepperVideoDecoderHardwareAccelerationBehaviorWithSWVD)
	default:
		s.Fatal("Unrecognized value for params.verifyHWMode: ", params.verifyHWMode)
	}

	// We pass a successCount equal to 2 because the Pepper plugin used in this test has two video decoders.
	expectedModeUsed, err := histogram.WasHWAccelUsed(ctx, ctconn, initHistogram, hwBehaviourHistogramName, hwBehaviourSucessValue, 2)
	if err != nil {
		s.Fatal("Failed to verify histogram: ", err)
	}

	if !expectedModeUsed {
		switch params.verifyHWMode {
		case verifyMojoVDPathWasUsed:
			s.Fatal("Hardware decoder backed by MojoVideoDecoder was not used")
		case verifyLegacyVDAPathWasUsed:
			s.Fatal("Hardware decoder backed by legacy VDA was not used")
		case verifySWPathWasUsed:
			s.Fatal("Software decoder was not used")
		}
	}

	// Check that the video finished without errors.
	if err := conn.WaitForExprWithTimeout(ctx, "doneTesting", 20*time.Second); err != nil {
		s.Fatal("The NaCl app never finished playing the video: ", err)
	}
}
