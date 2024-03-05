// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package youtube

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/input"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	mouseMoveDuration = 500 * time.Millisecond
	longUITimeout     = time.Minute
	shortUITimeout    = 5 * time.Second
)

var (
	videoPlayer = nodewith.NameStartingWith("YouTube Video Player").Role(role.GenericContainer)
	video       = nodewith.Role(role.Video).Ancestor(videoPlayer)
	videoButton = nodewith.Role(role.Button).Ancestor(videoPlayer).NameRegex(regexp.MustCompile("^(Pause|Play).*"))
)

// YtWeb defines the struct related to youtube web.
type YtWeb struct {
	br      *browser.Browser
	tconn   *chrome.TestConn
	kb      *input.KeyboardEventWriter
	ui      *uiauto.Context
	ytConn  *chrome.Conn
	ytWinID int
	uiHdl   cuj.UIActionHandler

	extendedDisplay bool
	openNewWindow   bool
}

// NewYtWeb creates an instance of YtWeb.
func NewYtWeb(br *browser.Browser, tconn *chrome.TestConn, kb *input.KeyboardEventWriter, extendedDisplay bool, ui *uiauto.Context, uiHdl cuj.UIActionHandler) *YtWeb {
	return &YtWeb{
		br:    br,
		tconn: tconn,
		kb:    kb,
		ui:    ui,
		uiHdl: uiHdl,

		extendedDisplay: extendedDisplay,
		openNewWindow:   true,
	}
}

// SetNewWindow sets new window for playing youtube video.
func (y *YtWeb) SetNewWindow(value bool) {
	y.openNewWindow = value
}

// Install installs the Youtube app with apk.
func (y *YtWeb) Install(ctx context.Context) error {
	return nil
}

// OpenAndPlayVideo opens a youtube video on chrome.
func (y *YtWeb) OpenAndPlayVideo(video VideoSrc) uiauto.Action {
	return func(ctx context.Context) (err error) {
		testing.ContextLog(ctx, "Open Youtube web")
		y.ytConn, err = y.uiHdl.NewChromeTab(ctx, y.br, video.URL, y.openNewWindow)
		if err != nil {
			return errors.Wrap(err, "failed to open youtube tab")
		}
		if err := webutil.WaitForYoutubeVideo(ctx, y.ytConn, longUITimeout); err != nil {
			return errors.Wrap(err, "failed to wait for video element")
		}
		// If prompted to open in YouTube app, instruct device to stay in Chrome.
		stayInChrome := nodewith.Name("Stay in Chrome").Role(role.Button)
		if err := uiauto.IfSuccessThen(
			y.ui.WithTimeout(shortUITimeout).WaitUntilExists(stayInChrome),
			func(ctx context.Context) error {
				testing.ContextLog(ctx, "dialog popped up and asked whether to switch to YouTube app")
				rememberReg := regexp.MustCompile("Remember (my|this) choice")
				rememberChoice := nodewith.NameRegex(rememberReg).Role(role.CheckBox)
				if err := y.uiHdl.Click(rememberChoice)(ctx); err != nil {
					return err
				}
				if err := y.uiHdl.Click(stayInChrome)(ctx); err != nil {
					return err
				}
				testing.ContextLog(ctx, "instructed device to stay on YouTube web")
				return nil
			},
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to instruct device to stay on YouTube web")
		}

		// In Lacros, it will focus on the search bar after navigating, click on the video title to make sure the focus is on the webarea.
		videoWebArea := nodewith.NameContaining(video.Title).Role(role.RootWebArea)
		videoWebAreaFocused := videoWebArea.Focused()
		videoTitle := nodewith.Name(video.Title).Role(role.Heading).Ancestor(videoWebArea)
		if err := uiauto.Combine("focus on video",
			y.clearNotificationPrompts,
			uiauto.IfFailThen(
				y.ui.Exists(videoWebAreaFocused),
				y.uiHdl.ClickUntil(
					videoTitle,
					y.ui.WithTimeout(shortUITimeout).WaitUntilExists(videoWebAreaFocused),
				),
			),
		)(ctx); err != nil {
			return err
		}

		// Use keyboard to play/pause video and ensure PageLoad.PaintTiming.NavigationToLargestContentfulPaint2
		// can be generated correctly. See b/240998447.
		if err := uiauto.Combine("pause and play with keyboard",
			y.Pause(),
			y.Play(),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to pause and play before switching quality")
		}

		// Sometimes prompts to grant permission appears after opening a video for a while.
		if err := y.clearNotificationPrompts(ctx); err != nil {
			return errors.Wrap(err, "failed to clear notification prompts")
		}

		// Default expected display is main display.
		if err := cuj.SwitchWindowToDisplay(ctx, y.tconn, y.kb, y.extendedDisplay)(ctx); err != nil {
			if y.extendedDisplay {
				return errors.Wrap(err, "failed to switch Youtube to the extended display")
			}
			return errors.Wrap(err, "failed to switch Youtube to the main display")
		}

		if err := y.SkipAd()(ctx); err != nil {
			return errors.Wrap(err, "failed to click 'Skip Ad' button")
		}

		if err := y.SwitchQuality(video.Quality)(ctx); err != nil {
			return errors.Wrapf(err, "failed to switch resolution to %s", video.Quality)
		}

		y.ytWinID, err = getFirstWindowID(ctx, y.tconn)
		if err != nil {
			return errors.Wrap(err, "failed to get window ID")
		}

		// Ensure the video is playing.
		return uiauto.IfFailThen(y.IsPlaying(), y.Play())(ctx)
	}
}

// SwitchQuality switches youtube quality.
func (y *YtWeb) SwitchQuality(quality Quality) uiauto.Action {
	return func(ctx context.Context) error {
		testing.ContextLog(ctx, "Switch video quality to ", quality)
		settings := nodewith.Name("Settings").Role(role.PopUpButton).Ancestor(videoPlayer)
		qualityFinder := nodewith.NameStartingWith("Quality").Role(role.MenuItem).Ancestor(videoPlayer)

		if err := y.ui.WaitUntilExists(videoPlayer)(ctx); err != nil {
			return errors.Wrap(err, "failed to find 'YouTube Video Player'")
		}

		startTime := time.Now()
		// The setting panel will automatically disappear if it does not receive any event after a few seconds.
		// Dut to the different response time of different DUTs, we need to combine these actions in Poll() to
		// make quality switch works reliably.
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			// If an ad is playing, skip it before proceeding.
			if err := y.SkipAd()(ctx); err != nil {
				return errors.Wrap(err, "failed to click 'Skip Ad' button")
			}

			// Use DoDefault to avoid fauilure on lacros (see bug b/229003599).
			if err := y.ui.DoDefault(settings)(ctx); err != nil {
				return errors.Wrap(err, "failed to call DoDefault on settings button")
			}
			if err := y.ui.WithTimeout(10 * time.Second).WaitUntilExists(qualityFinder)(ctx); err != nil {
				if y.extendedDisplay {
					return errors.Wrap(err, "failed to show the setting panel and click it on extended display")
				}
				return errors.Wrap(err, "failed to show the setting panel and click it on internal display")
			}

			testing.ContextLogf(ctx, "Elapsed time to click setting panel: %.3f s", time.Since(startTime).Seconds())
			return nil
		}, &testing.PollOptions{Interval: 3 * time.Second, Timeout: 30 * time.Second}); err != nil {
			return errors.Wrap(err, "failed to click setting panel")
		}

		// Use DoDefault to avoid fauilure on lacros (see bug b/229003599).
		if err := y.ui.DoDefault(qualityFinder)(ctx); err != nil {
			return errors.Wrap(err, "failed to click 'Quality'")
		}

		resolutionFinder := nodewith.NameStartingWith(string(quality)).Role(role.MenuItemRadio).Ancestor(videoPlayer)
		if err := y.ui.DoDefault(resolutionFinder)(ctx); err != nil {
			return errors.Wrapf(err, "failed to click %q", quality)
		}

		if err := waitForYoutubeReadyState(ctx, y.ytConn); err != nil {
			return errors.Wrap(err, "failed to wait for Youtube ready state")
		}

		// Keep the video playing anyway when switch the quality is finished.
		return uiauto.IfFailThen(y.IsPlaying(), y.Play())(ctx)
	}
}

// EnterFullScreen switches youtube video to full screen.
func (y *YtWeb) EnterFullScreen(ctx context.Context) error {
	// If the youtube app is already in full screen, skip the process to go fullscreen.
	if err := waitWindowStateFullscreen(y.tconn, YoutubeWindowTitle)(ctx); err == nil {
		return nil
	}

	fullscreenBtn := nodewith.Name("Full screen (f)").Role(role.Button)
	if err := uiauto.NamedCombine("make Youtube video fullscreen",
		// Notification prompts are sometimes shown in fullscreen.
		y.clearNotificationPrompts,
		y.ui.DoDefault(fullscreenBtn),
		waitWindowStateFullscreen(y.tconn, YoutubeWindowTitle),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to click full screen button")
	}

	if err := waitForYoutubeReadyState(ctx, y.ytConn); err != nil {
		return errors.Wrap(err, "failed to wait for Youtube ready state")
	}
	return nil
}

// ExitFullScreen exits Youtube video from full screen.
func (y *YtWeb) ExitFullScreen(ctx context.Context) error {
	// If the youtube app is already not in full screen, skip the process to exit fullscreen.
	if err := waitWindowStateExitFullscreen(y.tconn, YoutubeWindowTitle)(ctx); err == nil {
		return nil
	}

	// Move the mouse to a specific location to ensure focus on the video now.
	// Sometimes the button name will be "Exit full screen" or remain "Full screen" even when entering full screen.
	// It's more stable to use keyboard shortcuts.
	if err := uiauto.NamedCombine("exit Youtube from fullscreen",
		y.clearNotificationPrompts,
		y.ui.MouseMoveTo(videoButton, mouseMoveDuration),
		y.kb.AccelAction("f"),
		waitWindowStateExitFullscreen(y.tconn, YoutubeWindowTitle),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to exit fullscreen")
	}

	if err := waitForYoutubeReadyState(ctx, y.ytConn); err != nil {
		return errors.Wrap(err, "failed to wait for Youtube ready state")
	}
	return nil
}

// SkipAd skips the ad.
func (y *YtWeb) SkipAd() uiauto.Action {
	return func(ctx context.Context) error {
		if err := y.clearNotificationPrompts(ctx); err != nil {
			return err
		}

		testing.ContextLog(ctx, "Checking for YouTube ads")
		adClass := nodewith.HasClass("ytp-ad-player-overlay").Ancestor(videoPlayer).First()
		skipAdButton := nodewith.NameStartingWith("Skip").Role(role.Button).Ancestor(videoPlayer).First()
		return testing.Poll(ctx, func(ctx context.Context) error {
			if err := y.ui.WithTimeout(shortUITimeout).WaitUntilExists(adClass)(ctx); err != nil {
				testing.ContextLog(ctx, "No ads found")
				return nil
			}
			if err := y.ui.Exists(skipAdButton)(ctx); err != nil {
				return errors.Wrap(err, "'Skip Ads' button not available yet")
			}
			if err := y.uiHdl.Click(skipAdButton)(ctx); err != nil {
				return errors.Wrap(err, "failed to click 'Skip Ads'")
			}
			return errors.New("have not determined whether the ad has been skipped successfully")
		}, &testing.PollOptions{Timeout: longUITimeout})
	}
}

// MaximizeWindow maximizes the youtube video.
func (y *YtWeb) MaximizeWindow(ctx context.Context) error {
	testing.ContextLog(ctx, "Maximize Youtube video window")

	if ytWin, err := ash.GetWindow(ctx, y.tconn, y.ytWinID); err != nil {
		return errors.Wrap(err, "failed to get youtube window")
	} else if ytWin.State == ash.WindowStateMaximized {
		return nil
	}

	maximizeButton := nodewith.Name("Maximize").HasClass("FrameSizeButton").Role(role.Button)
	if err := y.uiHdl.Click(maximizeButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to maximize the window")
	}
	if err := ash.WaitForCondition(ctx, y.tconn, func(w *ash.Window) bool {
		return w.ID == y.ytWinID && w.State == ash.WindowStateMaximized && !w.IsAnimating
	}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to wait for window to become maximized")
	}
	return nil
}

// MinimizeWindow minimizes the youtube video.
func (y *YtWeb) MinimizeWindow(ctx context.Context) error {
	testing.ContextLog(ctx, "Minimize Youtube video window")

	if ytWin, err := ash.GetWindow(ctx, y.tconn, y.ytWinID); err != nil {
		return errors.Wrap(err, "failed to get youtube window")
	} else if ytWin.State == ash.WindowStateMinimized {
		return nil
	}

	minimizeButton := nodewith.Name("Minimize").HasClass("FrameCaptionButton").Role(role.Button)
	if err := y.uiHdl.Click(minimizeButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to minimize the window")
	}
	if err := ash.WaitForCondition(ctx, y.tconn, func(w *ash.Window) bool {
		return w.ID == y.ytWinID && w.State == ash.WindowStateMinimized && !w.IsAnimating
	}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to wait for window to become minimized")
	}
	return nil
}

// RestoreWindow restores the youtube video to normal state.
func (y *YtWeb) RestoreWindow(ctx context.Context) error {
	testing.ContextLog(ctx, "Restore Youtube video window")

	if _, err := ash.SetWindowState(ctx, y.tconn, y.ytWinID, ash.WMEventNormal, true /* waitForStateChange */); err != nil {
		return errors.Wrap(err, "failed to set the window state to normal")
	}
	if err := ash.WaitForCondition(ctx, y.tconn, func(w *ash.Window) bool {
		return w.ID == y.ytWinID && w.State == ash.WindowStateNormal && !w.IsAnimating
	}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to wait for window to become normal")
	}
	return nil
}

// PauseAndPlayVideo verifies video playback on youtube web.
func (y *YtWeb) PauseAndPlayVideo(ctx context.Context) error {
	return uiauto.NamedCombine("pause and play video",
		y.SkipAd(),
		// The video should be playing at this point. However, we'll double check to make sure
		// as we have seen a few cases where the video became paused automatically.
		uiauto.IfFailThen(y.IsPlaying(), y.Play()),
		y.Pause(),
		y.Play(),
	)(ctx)
}

// Play returns a function to play the video.
func (y *YtWeb) Play() uiauto.Action {
	return uiauto.NamedCombine("play video",
		y.clearNotificationPrompts,
		uiauto.IfSuccessThen(
			y.IsPaused(),
			y.ui.WithTimeout(longUITimeout).RetryUntil(y.kb.TypeAction("k"), y.IsPlaying()),
		),
	)
}

// Pause returns a function to pause the video.
func (y *YtWeb) Pause() uiauto.Action {
	return uiauto.NamedCombine("pause video",
		y.clearNotificationPrompts,
		uiauto.IfSuccessThen(
			y.IsPlaying(),
			y.ui.WithTimeout(longUITimeout).RetryUntil(y.kb.TypeAction("k"), y.IsPaused()),
		),
	)
}

const (
	playButton  = "Play (k)"
	pauseButton = "Pause (k)"
)

func (y *YtWeb) getPlayButtonTitle(ctx context.Context) (result string, err error) {
	script := `document.querySelector(".ytp-play-button").title`
	if err := y.ytConn.Eval(ctx, script, &result); err != nil {
		return result, errors.Wrap(err, "failed to get result")
	}
	return result, nil
}

// IsPlaying checks if the video is playing now.
func (y *YtWeb) IsPlaying() uiauto.Action {
	return func(ctx context.Context) error {
		return testing.Poll(ctx, func(ctx context.Context) error {
			title, err := y.getPlayButtonTitle(ctx)
			if err != nil {
				return err
			}
			if title == pauseButton {
				testing.ContextLog(ctx, "Youtube is playing")
				return nil
			}
			return errors.Errorf("youtube is not playing; got (%s)", title)
		}, &testing.PollOptions{Timeout: 10 * time.Second})
	}
}

// IsPaused checks if the video is paused now.
func (y *YtWeb) IsPaused() uiauto.Action {
	return func(ctx context.Context) error {
		return testing.Poll(ctx, func(ctx context.Context) error {
			title, err := y.getPlayButtonTitle(ctx)
			if err != nil {
				return err
			}
			if title == playButton {
				testing.ContextLog(ctx, "Youtube is paused")
				return nil
			}
			return errors.Errorf("youtube is not paused; got (%s)", title)
		}, &testing.PollOptions{Timeout: 10 * time.Second})
	}
}

// waitForYoutubeReadyState does wait youtube video ready state then return.
func waitForYoutubeReadyState(ctx context.Context, conn *chrome.Conn) error {
	startTime := time.Now()
	// Wait for element to appear.
	return testing.Poll(ctx, func(ctx context.Context) error {
		// Querying the main <video> node in youtube page.
		var state bool
		if err := conn.Call(ctx, &state, `() => {
			let video = document.querySelector("#movie_player > div.html5-video-container > video");
			return video.readyState === 4 && video.buffered.length > 0;
		}`); err != nil {
			return err
		}
		if !state {
			return errors.New("failed to wait for youtube on ready state")
		}
		testing.ContextLogf(ctx, "Elapsed time when waiting for youtube ready state: %.3f s", time.Since(startTime).Seconds())
		return nil
	}, &testing.PollOptions{Interval: time.Second, Timeout: longUITimeout})
}

// Close closes the resources related to video.
func (y *YtWeb) Close(ctx context.Context) {
	if y.ytConn != nil {
		y.ytConn.CloseTarget(ctx)
		y.ytConn.Close()
		y.ytConn = nil
	}
}

// clearNotificationPrompts finds and clears some youtube web prompts.
func (y *YtWeb) clearNotificationPrompts(ctx context.Context) error {
	tartgetPrompts := nodewith.NameRegex(regexp.MustCompile("(Allow|Never|NO THANKS|Got it)")).Role(role.Button)
	nodes, err := y.ui.NodesInfo(ctx, tartgetPrompts)
	if err != nil {
		return err
	}
	if len(nodes) == 0 {
		return nil
	}

	testing.ContextLog(ctx, "Start to clear notification prompts")
	prompts := []string{"Allow", "Never", "NO THANKS", "Got it"}
	for _, name := range prompts {
		tartgetPrompt := nodewith.Name(name).Role(role.Button)
		if err := uiauto.IfSuccessThen(
			y.ui.WithTimeout(shortUITimeout).WaitUntilExists(tartgetPrompt),
			y.ui.DoDefaultUntil(tartgetPrompt, y.ui.WithTimeout(shortUITimeout).WaitUntilGone(tartgetPrompt)),
		)(ctx); err != nil {
			testing.ContextLogf(ctx, "Failed to clear prompt %q", name)
			return err
		}
	}
	return nil
}

// PerformFrameDropsTest checks for dropped frames percent and checks if it is below the threshold.
func (y *YtWeb) PerformFrameDropsTest(ctx context.Context) error {
	// If we see more than 10% video frame drops it will be visible to the user and will impact the viewing experience.
	const frameDropThreshold float64 = 10.0
	var decodedFrameCount, droppedFrameCount int
	videoElement := "document.querySelector('#movie_player video')"

	if err := y.ytConn.Eval(ctx, videoElement+".getVideoPlaybackQuality().totalVideoFrames", &decodedFrameCount); err != nil {
		return errors.Wrap(err, "failed to get decoded framecount")
	}
	if err := y.ytConn.Eval(ctx, videoElement+".getVideoPlaybackQuality().droppedVideoFrames", &droppedFrameCount); err != nil {
		return errors.Wrap(err, "failed to get dropped framecount")
	}
	droppedFramePercent := 100.0
	if decodedFrameCount != 0 {
		droppedFramePercent = 100.0 * float64(droppedFrameCount/decodedFrameCount)

	}
	if droppedFramePercent > frameDropThreshold {
		return errors.Errorf("frame drops rate %.2f (dropped %d, decoded %d) higher than allowed threshold %.2f", droppedFramePercent, droppedFrameCount, decodedFrameCount, frameDropThreshold)
	}
	return nil
}

// YtWebConn returns connection of youtube web.
func (y *YtWeb) YtWebConn() *chrome.Conn {
	return y.ytConn
}
