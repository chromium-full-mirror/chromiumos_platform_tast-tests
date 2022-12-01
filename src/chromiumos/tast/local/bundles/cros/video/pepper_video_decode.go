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
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PepperVideoDecode,
		LacrosStatus: testing.LacrosVariantUnknown,
		Desc:         "Checks that simple video playback in Pepper (NaCl) is working",
		Contacts: []string{
			"pmolinalopez@chromium.org",
			"chromeos-gfx-video@google.com",
		},
		Data: []string{
			"pepper/video_decode/pnacl/Release/video_decode.nmf",
			"pepper/video_decode/pnacl/Release/video_decode.pexe",
			"pepper/video_decode/video_decode.html",
		},
		SoftwareDeps: []string{"chrome", "nacl"},
		Params: []testing.Param{{
			Name:              "h264_hw",
			Val:               browser.TypeAsh,
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideoNaCl",
		}, {
			Name:              "h264_hw_mojovd",
			Val:               browser.TypeAsh,
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideoNaClWithMojoVideoDecoder",
		}, {
			Name:              "h264_hw_nopepper3dimage",
			Val:               browser.TypeAsh,
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideoNaClWithoutPepper3DImage",
		}, {
			Name:              "h264_hw_mojovd_nopepper3dimage",
			Val:               browser.TypeAsh,
			ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideoNaClWithMojoVideoDecoderWithoutPepper3DImage",
		}},
	})
}

func PepperVideoDecode(ctx context.Context, s *testing.State) {
	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	cr := s.FixtValue().(*chrome.Chrome)
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
	ctconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}
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

	// Check that the video finished without errors.
	if err := conn.WaitForExprWithTimeout(ctx, "doneTesting", 20*time.Second); err != nil {
		s.Fatal("The NaCl app never finished playing the video: ", err)
	}
}
