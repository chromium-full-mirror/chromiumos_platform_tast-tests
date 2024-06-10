// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"context"
	"net/url"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/media/devtools"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	crosAppspotH264VanillaURL            = "https://crosvideo.appspot.com/?codec=h264&resolution=720&loop=true&mute=true"
	crosAppspotVP9VanillaURL             = "https://crosvideo.appspot.com/?codec=vp9&resolution=720&loop=true&mute=true"
	crosAppspotH264ChangingResolutionURL = "https://crosvideo.appspot.com/?codec=h264&cycle=true&loop=true&mute=true"
	crosAppspotVP9ChangingResolutionURL  = "https://crosvideo.appspot.com/?codec=vp9&cycle=true&loop=true&mute=true"

	// From b/342022288.
	widevineClearURL = "https://integration.uat.widevine.com/player?autoPlay=true&contentUrl=https://storage.googleapis.com/wvmedia/clear/vp9/30fps/llama/llama_uhd.mpd"

	// Whatever URL this test navigates to, it should have an element with id
	// "video", to monitor its |currentTime| attribute.
	videoElement = "document.getElementsByTagName('video')[0]"

	// The interval at which the test verifies that |videoElement| is playing.
	samplingInterval = 20 * time.Second
)

type playURLParams struct {
	url         string
	browserType browser.Type
	duration    time.Duration
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         PlayURL,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Plays video in Chrome by navigating to a given URL, and verifies it plays",
		Contacts: []string{
			"chromeos-gfx-video@google.com",
			"mcasas@chromium.org",
		},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{
			{
				Name: "crosvideo_h264_720_1minute",
				Val: playURLParams{
					url:         crosAppspotH264VanillaURL,
					duration:    1 * time.Minute,
					browserType: browser.TypeAsh,
				},
				ExtraSoftwareDeps: []string{"proprietary_codecs", caps.HWDecodeH264},
				ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
				Fixture:           "chromeVideo",
				Timeout:           5 * time.Minute,
			},
			{
				Name: "crosvideo_vp9_720_1minute",
				Val: playURLParams{
					url:         crosAppspotVP9VanillaURL,
					duration:    1 * time.Minute,
					browserType: browser.TypeAsh,
				},
				ExtraSoftwareDeps: []string{caps.HWDecodeVP9},
				ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
				Fixture:           "chromeVideo",
				Timeout:           5 * time.Minute,
			},
			{
				Name: "v4l2_flat_crosvideo_h264_720_1minute",
				Val: playURLParams{
					url:         crosAppspotH264VanillaURL,
					duration:    1 * time.Minute,
					browserType: browser.TypeAsh,
				},
				ExtraSoftwareDeps: []string{"proprietary_codecs", caps.HWDecodeH264},
				ExtraHardwareDeps: hwdep.D(hwdep.SupportsV4L2FlatVideoDecoding()),
				ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
				Fixture:           "chromeVideoWithV4L2FlatDecoder",
				Timeout:           5 * time.Minute,
			},
			{
				Name: "v4l2_flat_crosvideo_vp9_720_1minute",
				Val: playURLParams{
					url:         crosAppspotVP9VanillaURL,
					duration:    1 * time.Minute,
					browserType: browser.TypeAsh,
				},
				ExtraSoftwareDeps: []string{caps.HWDecodeVP9},
				ExtraHardwareDeps: hwdep.D(hwdep.SupportsV4L2FlatVideoDecoding()),
				ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
				Fixture:           "chromeVideoWithV4L2FlatDecoder",
				Timeout:           5 * time.Minute,
			},
			{
				Name: "crosvideo_h264_cycle_1minute",
				Val: playURLParams{
					url:         crosAppspotH264ChangingResolutionURL,
					duration:    1 * time.Minute,
					browserType: browser.TypeAsh,
				},
				ExtraSoftwareDeps: []string{"proprietary_codecs", caps.HWDecodeH264},
				ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
				Fixture:           "chromeVideo",
				Timeout:           5 * time.Minute,
			},
			{
				Name: "crosvideo_h264_cycle_1minute_gtfo",
				Val: playURLParams{
					url:         crosAppspotH264ChangingResolutionURL,
					duration:    1 * time.Minute,
					browserType: browser.TypeAsh,
				},
				ExtraSoftwareDeps: []string{"proprietary_codecs", caps.HWDecodeH264},
				ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
				Fixture:           "chromeVideoGTFO",
				Timeout:           5 * time.Minute,
			},
			{
				Name: "crosvideo_vp9_cycle_1minute",
				Val: playURLParams{
					url:         crosAppspotVP9ChangingResolutionURL,
					duration:    1 * time.Minute,
					browserType: browser.TypeAsh,
				},
				ExtraSoftwareDeps: []string{caps.HWDecodeVP9},
				ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
				Fixture:           "chromeVideo",
				Timeout:           5 * time.Minute,
			},
			{
				Name: "v4l2_flat_crosvideo_h264_cycle_1minute",
				Val: playURLParams{
					url:         crosAppspotH264ChangingResolutionURL,
					duration:    1 * time.Minute,
					browserType: browser.TypeAsh,
				},
				ExtraSoftwareDeps: []string{"proprietary_codecs", caps.HWDecodeH264},
				ExtraHardwareDeps: hwdep.D(hwdep.SupportsV4L2FlatVideoDecoding()),
				ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
				Fixture:           "chromeVideoWithV4L2FlatDecoder",
				Timeout:           5 * time.Minute,
			},
			{
				Name: "v4l2_flat_crosvideo_vp9_cycle_1minute",
				Val: playURLParams{
					url:         crosAppspotVP9ChangingResolutionURL,
					duration:    1 * time.Minute,
					browserType: browser.TypeAsh,
				},
				ExtraSoftwareDeps: []string{caps.HWDecodeVP9},
				ExtraHardwareDeps: hwdep.D(hwdep.SupportsV4L2FlatVideoDecoding()),
				ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_perbuild"},
				Fixture:           "chromeVideoWithV4L2FlatDecoder",
				Timeout:           5 * time.Minute,
			},
			{
				Name: "widevine_clear_vp9_2minute",
				Val: playURLParams{
					url:         widevineClearURL,
					duration:    2 * time.Minute,
					browserType: browser.TypeAsh,
				},
				ExtraSoftwareDeps: []string{caps.HWDecodeVP9_4K},
				ExtraAttr:         []string{"group:graphics", "graphics_video", "graphics_weekly"},
				Fixture:           "chromeVideo",
				Timeout:           5 * time.Minute,
			},
		},
	})
}

// PlayURL navigates to a given URL, that presumably has at least one <video>
// element inside, and verifies video is playing correctly via e.g. increasing
// played timestamp.
func PlayURL(ctx context.Context, s *testing.State) {
	testOpt := s.Param().(playURLParams)

	cr, l, _, err := lacros.Setup(ctx, s.FixtValue(), testOpt.browserType)
	if err != nil {
		s.Fatal("Failed to initialize test: ", err)
	}
	defer lacros.CloseLacros(ctx, l)

	var br *browser.Browser
	switch testOpt.browserType {
	case browser.TypeAsh:
		br = cr.Browser()
	case browser.TypeLacros:
		br = l.Browser()
	}

	// Navigate to the interesting URL and maximize the window.
	conn, err := br.NewConn(ctx, testOpt.url)
	if err != nil {
		s.Fatal("Failed to create browser connection: ", err)
	}
	defer conn.Close()
	defer conn.CloseTarget(ctx)

	observer, err := conn.GetMediaPropertiesChangedObserver(ctx)
	if err != nil {
		s.Fatal("Failed to retrieve DevTools Media messages: ", err)
	}

	// See e.g. [1] for readyState details.
	// [1] https://developer.mozilla.org/en-US/docs/Web/API/HTMLMediaElement/readyState
	if err := conn.WaitForExprWithTimeout(ctx, videoElement+".readyState > 2", samplingInterval); err != nil {
		s.Fatal("Video failed to start playing: ", err)
	}

	u, _ := url.Parse(testOpt.url)
	isPlatform, _, err := devtools.GetVideoDecoder(ctx, observer, u.Host+u.Path)
	if err != nil {
		s.Fatal("Failed to parse Media DevTools: ", err)
	}
	if !isPlatform {
		s.Fatal("Hardware decoding accelerator was expected but wasn't used")
	}

	// Make sure |videoElement| is playing every so often.
	previousPlayTime := -1.0
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		ended := false
		if err := conn.Eval(ctx, videoElement+".ended", &ended); err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get <video> 'ended' property"))
		} else if ended {
			testing.ContextLog(ctx, "<video> element has finished playing")
			return nil
		}

		var lastPlayedTime float64
		if err := conn.Eval(ctx, videoElement+".currentTime", &lastPlayedTime); err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get <video> current playing time"))
		}

		if lastPlayedTime == previousPlayTime {
			return testing.PollBreak(errors.Errorf("<video> failed to progress from %v", lastPlayedTime))
		}
		previousPlayTime = lastPlayedTime

		if lastPlayedTime < testOpt.duration.Seconds() {
			return errors.New("Keep checking that <video> is still playing")
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  2 * testOpt.duration,
		Interval: samplingInterval,
	}); err != nil {
		s.Fatal("Video seems to be stuck: ", err)
	}
}
