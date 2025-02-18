// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type webCodecsConfig struct {
	isDecoder bool
	configStr string
}

func init() {
	testing.AddTest(&testing.Test{
		Func: WebCodecsConfigure,
		Desc: "Verifies WebCodecs supports desired configurations",
		Contacts: []string{
			"chromeos-gfx-video@google.com",
			"hiroh@chromium.org",
		},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
		Params: []testing.Param{{
			Name: "zoom_enc",
			Val: webCodecsConfig{
				isDecoder: false,
				configStr: `{
					codec: "avc1.640028",
					bitrate: 1500000,
					width: 1280,
					height: 720,
					avc: { "format": "annexb" },
					framerate:25,
					hardwareAcceleration: "no-preference",
					latencyMode: "realtime",
					bitrateMode: "constant",
					scalabilityMode:"L1T2",
				}`,
			},
			Fixture: "chromeVideo",
		}, {
			Name: "zoom_enc_hw",
			Val: webCodecsConfig{
				isDecoder: false,
				configStr: `{
					codec: "avc1.640028",
					bitrate: 1500000,
					width: 1280,
					height: 720,
					avc: { "format": "annexb" },
					framerate:25,
					hardwareAcceleration: "prefer-hardware",
					latencyMode: "realtime",
					bitrateMode: "constant",
					scalabilityMode:"L1T2",
				}`,
			},
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsSVCEncoding("h264baseline", "l1t2")),
			ExtraSoftwareDeps: []string{caps.HWEncodeH264},
			Fixture:           "chromeVideoHardwareTemporalEncoding",
		}, {
			Name: "zoom_dec_hw",
			Val: webCodecsConfig{
				isDecoder: true,
				configStr: `{
					codec: "avc1.640028",
					codedWidth: 1280,
					codedHeight: 720,
					hardwareAcceleration: "prefer-hardware",
					optimizeForLatency: true
				}`,
			},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264},
			Fixture:           "chromeVideo",
		}},
	})
}

func WebCodecsConfigure(ctx context.Context, s *testing.State) {
	args := s.Param().(webCodecsConfig)
	webCodecsClass := "VideoEncoder"
	if args.isDecoder {
		webCodecsClass = "VideoDecoder"
	}
	webCodecsConfigStr := args.configStr
	webCodecsIsConfigSupported := fmt.Sprintf(`async() => {
		let config = %s;
		let ret = await %s.isConfigSupported(config);
		return ret.supported;
	}`, webCodecsConfigStr, webCodecsClass)

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	conn, err := cr.NewConn(ctx, server.URL)
	if err != nil {
		s.Fatal("Failed to open blank page: ", err)
	}
	defer conn.Close()

	var success bool
	if err := conn.Call(ctx, &success, webCodecsIsConfigSupported); err != nil {
		s.Fatal("Execute webcodecs API fails: ", err)
	}

	if !success {
		s.Fatal("The configuration is not supported")
	}
}
