// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package webrtc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast/core/testing"
)

type videoPlaybackDelayParams struct {
	profile     string
	browserType browser.Type
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         VideoPlaybackDelay,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Runs a webrtc playback-only connection to get performance numbers",
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video
		Contacts: []string{
			"chromeos-gfx@google.com",
			"mcasas@chromium.org",
		},
		Attr:         []string{"group:graphics", "graphics_video", "graphics_nightly"},
		SoftwareDeps: []string{"chrome", "no_qemu"},
		Data:         []string{"webrtc_video_display_perf_test.html", "third_party/munge_sdp.js"},
		Params: []testing.Param{{
			Name:              "vp8",
			Val:               videoPlaybackDelayParams{profile: "VP8", browserType: browser.TypeAsh},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP8},
			Fixture:           "chromeVideoWithFakeWebcamAndZeroLatencyRtc",
		}, {
			Name:              "vp9",
			Val:               videoPlaybackDelayParams{profile: "VP9", browserType: browser.TypeAsh},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9},
			Fixture:           "chromeVideoWithFakeWebcamAndZeroLatencyRtc",
		}, {
			Name:              "h264",
			Val:               videoPlaybackDelayParams{profile: "H264", browserType: browser.TypeAsh},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs"},
			Fixture:           "chromeVideoWithFakeWebcamAndZeroLatencyRtc",
		}, {
			Name:              "vp8_lacros",
			Val:               videoPlaybackDelayParams{profile: "VP8", browserType: browser.TypeLacros},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP8, "lacros"},
			Fixture:           "chromeVideoLacrosWithFakeWebcamAndZeroLatencyRtc",
		}, {
			Name:              "vp9_lacros",
			Val:               videoPlaybackDelayParams{profile: "VP9", browserType: browser.TypeLacros},
			ExtraSoftwareDeps: []string{caps.HWDecodeVP9, "lacros"},
			Fixture:           "chromeVideoLacrosWithFakeWebcamAndZeroLatencyRtc",
		}, {
			Name:              "h264_lacros",
			Val:               videoPlaybackDelayParams{profile: "H264", browserType: browser.TypeLacros},
			ExtraSoftwareDeps: []string{caps.HWDecodeH264, "proprietary_codecs", "lacros"},
			Fixture:           "chromeVideoLacrosWithFakeWebcamAndZeroLatencyRtc",
		}},
	})
}

func VideoPlaybackDelay(ctx context.Context, s *testing.State) {
	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()
	testURL := path.Join(server.URL, "webrtc_video_display_perf_test.html")

	testOpt := s.Param().(videoPlaybackDelayParams)
	cr, l, cs, err := lacros.Setup(ctx, s.FixtValue(), testOpt.browserType)
	if err != nil {
		s.Fatal("Failed to initialize test: ", err)
	}
	defer lacros.CloseLacros(ctx, l)

	conn, err := cs.NewConn(ctx, testURL)
	if err != nil {
		s.Fatalf("Failed to open %s: %v", testURL, err)
	}
	defer conn.Close()
	defer conn.CloseTarget(ctx)

	// We could consider removing the CPU frequency scaling and thermal throttling
	// to get more consistent results, but then we wouldn't be measuring on the
	// same conditions as a user might encounter.

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	var br *browser.Browser
	switch testOpt.browserType {
	case browser.TypeAsh:
		br = cr.Browser()
	case browser.TypeLacros:
		br = l.Browser()
	}
	bTconn, err := br.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to browser test API: ", err)
	}

	const presentationsHistogramName = "Media.VideoFrameSubmitter"
	initPresentationHistogram, err := metrics.GetHistogram(ctx, bTconn, presentationsHistogramName)
	if err != nil {
		s.Fatal("Failed to get initial histogram: ", err)
	}
	const decodeHistogramName = "Media.MojoVideoDecoder.Decode"
	initDecodeHistogram, err := metrics.GetHistogram(ctx, bTconn, decodeHistogramName)
	if err != nil {
		s.Fatal("Failed to get initial histogram: ", err)
	}
	const platformDecodeHistogramName = "Media.PlatformVideoDecoding.Decode"
	initPlatformDecodeHistogramName, err := metrics.GetHistogram(ctx, tconn, platformDecodeHistogramName)
	if err != nil {
		s.Fatal("Failed to get initial histogram: ", err)
	}

	profile := testOpt.profile
	if err := conn.Call(ctx, nil, `(profile) => new Promise(async (resolve, reject) => {
		  let pc1 = new RTCPeerConnection();
		  let pc2 = new RTCPeerConnection();

		  pc1.onicecandidate = e => pc2.addIceCandidate(e.candidate).catch(reject);
		  pc2.onicecandidate = e => pc1.addIceCandidate(e.candidate).catch(reject);
		  pc2.ontrack = e => {
		    let remoteVideo = document.getElementById('remoteVideo');
		    remoteVideo.srcObject = e.streams[0];
		    resolve();
		  };

		  try {
		    let stream = await navigator.mediaDevices.getUserMedia({
		      audio: false,
		      video: { width: 1920, height: 1080 }
		    });
		    stream.getTracks().forEach(track => pc1.addTrack(track, stream));
		    let offer1 = await pc1.createOffer();
		    if (profile) {
		      offer1.sdp = setSdpDefaultVideoCodec(offer1.sdp, profile, false, "");
		    }
		    await pc1.setLocalDescription(offer1);
		    await pc2.setRemoteDescription(pc1.localDescription);
		    let offer2 = await pc2.createAnswer();
		    await pc2.setLocalDescription(offer2);
		    await pc1.setRemoteDescription(pc2.localDescription);
		  } catch (e) {
		    reject(e);
		  }
		})`, profile); err != nil {
		s.Fatal("RTCPeerConnection establishment failed: ", err)
	}

	const playbackTime = 20 * time.Second
	// GoBigSleepLint: There's no easy way to count the amount of frames played back
	// by a <video> element, so let the connection roll with a timeout. At the
	// expected 30-60 fps, we need tens of seconds to accumulate a couple of hundred
	// frames to make the histograms significant.
	if err := testing.Sleep(ctx, playbackTime); err != nil {
		s.Fatal("Error while waiting for playback delay perf collection: ", err)
	}

	perfValues := perf.NewValues()
	if err := graphics.UpdatePerfMetricFromHistogram(ctx, bTconn, presentationsHistogramName, initPresentationHistogram, perfValues, "tast_graphics_webrtc_video_playback_delay"); err != nil {
		s.Fatal("Failed to calculate Presentation perf metric: ", err)
	}
	if err := graphics.UpdatePerfMetricFromHistogram(ctx, bTconn, decodeHistogramName, initDecodeHistogram, perfValues, "tast_graphics_webrtc_video_decode_delay"); err != nil {
		s.Fatal("Failed to calculate Decode perf metric: ", err)
	}
	if err := graphics.UpdatePerfMetricFromHistogram(ctx, tconn, platformDecodeHistogramName, initPlatformDecodeHistogramName, perfValues, "tast_graphics_webrtc_platform_video_decode_delay"); err != nil {
		s.Fatal("Failed to calculate Platform Decode perf metric: ", err)
	}

	if err = perfValues.Save(s.OutDir()); err != nil {
		s.Error("Cannot save perf data: ", err)
	}
}
