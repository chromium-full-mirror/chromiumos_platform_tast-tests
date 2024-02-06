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

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast-tests/cros/local/media/constants"
	"go.chromium.org/tast-tests/cros/local/media/histogram"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type verifyHWAcceleratorMode int

const (
	// verifyMojoVDPathWasUsed is a mode that verifies that a hardware decoder backed by the the newer MojoVideoDecoder path was used.
	verifyMojoVDPathWasUsed verifyHWAcceleratorMode = iota
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
		LacrosStatus: testing.LacrosVariantExists,
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
		Attr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
		Params: []testing.Param{{
			Name:              "h264_hw",
			Val:               pepperVideoDecodeTestParam{browserType: browser.TypeAsh, verifyHWMode: verifyMojoVDPathWasUsed},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideoNaCl",
		}, {
			Name:              "h264_sw",
			Val:               pepperVideoDecodeTestParam{browserType: browser.TypeAsh, verifyHWMode: verifySWPathWasUsed},
			ExtraSoftwareDeps: []string{"proprietary_codecs"},
			Fixture:           "chromeVideoNaClWithSWDecoding",
		}, {
			Name:              "h264_hw_lacros",
			Val:               pepperVideoDecodeTestParam{browserType: browser.TypeLacros, verifyHWMode: verifyMojoVDPathWasUsed},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs", "lacros"},
			Fixture:           "chromeVideoLacrosNaCl",
		}, {
			Name:              "h264_sw_lacros",
			Val:               pepperVideoDecodeTestParam{browserType: browser.TypeLacros, verifyHWMode: verifySWPathWasUsed},
			ExtraSoftwareDeps: []string{"proprietary_codecs", "lacros"},
			Fixture:           "chromeVideoLacrosNaClWithSWDecoding",
		}},
	})
}

func PepperVideoDecode(ctx context.Context, s *testing.State) {
	params := s.Param().(pepperVideoDecodeTestParam)

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	ctconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, params.browserType)
	if err != nil {
		s.Fatal("Failed to open the browser: ", err)
	}
	defer closeBrowser(cleanupCtx)

	bTconn, err := br.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to browser test API: ", err)
	}

	hwBehaviourHistogramName := constants.MediaPepperVideoDecoderHardwareAccelerationBehavior
	initHistogram, err := metrics.GetHistogram(ctx, bTconn, hwBehaviourHistogramName)
	if err != nil {
		s.Fatal("Failed to get initial histogram: ", err)
	}

	url := path.Join(server.URL, "pepper/video_decode/video_decode.html")
	conn, err := br.NewConn(ctx, url)
	if err != nil {
		s.Fatalf("Failed to open %v: %v", url, err)
	}
	defer conn.Close()

	// Check that the NaCl video decoding example loaded correctly.
	if err := conn.WaitForExprWithTimeout(ctx, "doneLoadingExample", 50*time.Second); err != nil {
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
	if params.verifyHWMode == verifyMojoVDPathWasUsed {
		hwBehaviourSucessValue = int64(constants.MediaPepperVideoDecoderHardwareAccelerationBehaviorWithMojoVD)
	} else if params.verifyHWMode == verifySWPathWasUsed {
		hwBehaviourSucessValue = int64(constants.MediaPepperVideoDecoderHardwareAccelerationBehaviorWithSWVD)
	} else {
		s.Fatal("Unrecognized value for params.verifyHWMode: ", params.verifyHWMode)
	}

	// We pass a successCount equal to 2 because the Pepper plugin used in this test has two video decoders.
	expectedModeUsed, err := histogram.WasHWAccelUsed(ctx, bTconn, initHistogram, hwBehaviourHistogramName, hwBehaviourSucessValue, 2)
	if err != nil {
		s.Fatal("Failed to verify histogram: ", err)
	}

	if !expectedModeUsed {
		if params.verifyHWMode == verifyMojoVDPathWasUsed {
			s.Fatal("Hardware decoder backed by MojoVideoDecoder was not used")
		} else if params.verifyHWMode == verifySWPathWasUsed {
			s.Fatal("Software decoder was not used")
		}
	}

	// Check that the video finished without errors.
	if err := conn.WaitForExprWithTimeout(ctx, "doneTesting", 20*time.Second); err != nil {
		s.Fatal("The NaCl app never finished playing the video: ", err)
	}
}
