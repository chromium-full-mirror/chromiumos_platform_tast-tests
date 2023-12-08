// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package ambient supports interaction with ChromeOS Ambient mode.
package ambient

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/checked"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/mtbf/youtube"
	"go.chromium.org/tast-tests/cros/local/personalization"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
)

// const for ambient topic sources.
const (
	GooglePhotos = "Google Photos"
	ArtGallery   = "Art gallery"
	VideoSource  = "Dawn to dark"
	OnStatus     = "On"
	OffStatus    = "Off"
)

// const for ambient themes.
const (
	SlideShow     = "Slide show"
	FeelTheBreeze = "Feel the breeze"
	FloatOnBy     = "Float on by"
	VideoTheme    = "Dawn to dark - exclusive"
)

// const for ambient video choices.
const (
	CloudsVideoName    = "Cloud Flow"
	NewMexicoVideoName = "Earth Flow"
)

// DefaultVideoName specifies the video selected by default when the video theme
// is active.
const DefaultVideoName = NewMexicoVideoName

// AlbumSelectedClassName specifies the label given to ambient albums that have
// been selected in the hub.
const AlbumSelectedClassName = "album-selected"

// Default values for TestParams' fields.
const (
	// Slide show mode only requires downloading and decoding 2 photos before the
	// screen saver starts rendering. The animations, however, can request around
	// 16 photos before starting (with an internal timeout after which screen
	// saver starts anyways if it can't prepare all 16). Since this is
	// significantly more than slide show, the timeout should be larger.
	StartSlideShowDefaultTimeout = 15 * time.Second
	StartAnimationDefaultTimeout = 30 * time.Second
	// Video files by design should almost always be cached on disc by the time a
	// screen saver session starts, leading to zero delay in rendering. In corner
	// cases where it's not, it should be downloaded immediately before rendering
	// can start, which should take at most ~15 seconds.
	StartVideoDefaultTimeout = 15 * time.Second
	// Typical animation cycle duration is currently 60 seconds. 60 seconds / 20
	// = 3 second cycle duration. This should give the test ample time to iterate
	// through 2 full animation cycles, giving it sufficient test coverage.
	AnimationFastForwardPlaybackSpeed = 20
)

// TestVideoSrc contains an example Youtube video to play for testing
// "media string" in ambient mode. Taken from the BasicYoutubeCUJ test.
var TestVideoSrc = youtube.VideoSrc{
	URL:     "https://www.youtube.com/watch?v=LXb3EKWsInQ",
	Title:   "COSTA RICA IN 4K 60fps HDR (ULTRA HD)",
	Quality: "1080p60",
}

// TestParams for each test case.
type TestParams struct {
	TopicSource string
	Theme       string
	// Amount of time to wait for an ambient session to start (can include things
	// like downloading photos, etc). Applies to all themes.
	StartupTimeout time.Duration
	// Whether we want to test the media player in ambient mode. Note this is not
	// the same as the `VideoTheme`. This tests the ambient media string (ex:
	// launch ambient mode with a YT video playing in the background). It is not
	// theme-specific.
	PlayTestVideo bool

	// Only used if the `Theme` is `VideoTheme`. Specifies the "album" name of
	// video to play.
	VideoThemeAlbum string
	// Only used if the `Theme` is a Lottie-animated theme. Specifies the
	// animation's playback rate so that the test can run faster.
	AnimationPlaybackSpeed float32
}

// DeviceSettings that must be set on the DUT before the test begins. These
// settings are not user-visible and can only be changed by test code. The main
// overarching purpose of these is to speed up ambient mode so that the test
// doesn't have to take too long.
type DeviceSettings struct {
	LockScreenIdle         time.Duration
	BackgroundLockScreen   time.Duration
	PhotoRefreshInterval   time.Duration
	AnimationPlaybackSpeed float32
}

// A node finder to turn Ambient off and on in Personalization App.
var ambientToggleButton *nodewith.Finder = nodewith.Role(role.ToggleButton).ClassName("clickable").NameStartingWith("When your screen is idle")

func toNearestSecond(d time.Duration) int {
	return int(d.Round(time.Second).Seconds())
}

// SetDeviceSettings changes settings for Ambient mode to speed up testing.
// Rounds values to the nearest second.
func SetDeviceSettings(tconn *chrome.TestConn, deviceSettings DeviceSettings) uiauto.Action {
	return func(ctx context.Context) error {
		if err := tconn.Call(
			ctx,
			nil,
			`tast.promisify(chrome.settingsPrivate.setPref)`,
			"ash.ambient.lock_screen_idle_timeout",
			toNearestSecond(deviceSettings.LockScreenIdle),
		); err != nil {
			return errors.Wrap(err, "failed to set lock screen idle timeout")
		}

		if err := tconn.Call(
			ctx,
			nil,
			`tast.promisify(chrome.settingsPrivate.setPref)`,
			"ash.ambient.lock_screen_background_timeout",
			toNearestSecond(deviceSettings.BackgroundLockScreen),
		); err != nil {
			return errors.Wrap(err, "failed to set lock screen background timeout")
		}

		if err := tconn.Call(
			ctx,
			nil,
			`tast.promisify(chrome.settingsPrivate.setPref)`,
			"ash.ambient.photo_refresh_interval",
			toNearestSecond(deviceSettings.PhotoRefreshInterval),
		); err != nil {
			return errors.Wrap(err, "failed to set photo refresh interval")
		}

		// 0 means the field was default initialized and is not intended to be used.
		if deviceSettings.AnimationPlaybackSpeed > 0 {
			if err := tconn.Call(
				ctx,
				nil,
				`tast.promisify(chrome.settingsPrivate.setPref)`,
				"ash.ambient.animation_playback_speed",
				deviceSettings.AnimationPlaybackSpeed,
			); err != nil {
				return errors.Wrap(err, "failed to set playback speed")
			}
		}
		return nil
	}
}

// OpenAmbientSubpage returns an action to open Personalization App and then ambient subpage.
func OpenAmbientSubpage(ui *uiauto.Context) uiauto.Action {
	return uiauto.Combine("open Ambient Subpage",
		personalization.OpenPersonalizationHub(ui),
		personalization.OpenScreenSaverSubpage(ui),
		ui.WaitUntilExists(personalization.BreadcrumbNodeFinder(personalization.ScreenSaverSubpageName)),
	)
}

// toggleAmbientMode returns an action to toggle ambient mode in ambient subpage.
func toggleAmbientMode(currentMode string, ui *uiauto.Context) uiauto.Action {
	return uiauto.Combine(fmt.Sprintf("toggle ambient mode - %s", currentMode),
		ui.WaitUntilExists(ambientToggleButton),
		ui.LeftClick(ambientToggleButton))
}

// ambientModeEnabled checks whether ambient mode is on.
func ambientModeEnabled(ctx context.Context, ui *uiauto.Context) (bool, error) {
	if err := ui.WaitUntilExists(ambientToggleButton)(ctx); err != nil {
		return false, errors.Wrap(err, "failed to find ambient toggle button")
	}
	info, err := ui.Info(ctx, ambientToggleButton)
	if err != nil {
		return false, errors.Wrap(err, "failed to get ambient toggle button info")
	}
	return info.Checked == checked.True, nil
}

// EnableAmbientMode enables ambient mode in Personalization Hub from Ambient Subpage.
// If ambient mode is already enabled, it does nothing.
func EnableAmbientMode(ui *uiauto.Context) uiauto.Action {
	return func(ctx context.Context) error {
		ambientMode, err := ambientModeEnabled(ctx, ui)
		if err != nil {
			return errors.Wrap(err, "failed to check ambient mode status")
		}
		if !ambientMode {
			if err := toggleAmbientMode(OffStatus, ui)(ctx); err != nil {
				return errors.Wrap(err, "failed to enable ambient mode")
			}
		}
		return nil
	}
}

// waitForPhotoTransitions blocks until the desired number of photo transitions
// has occurred.
func waitForPhotoTransitions(
	ctx context.Context,
	tconn *chrome.TestConn,
	numCompletions int,
	timeout time.Duration,
) error {
	return tconn.Call(
		ctx,
		nil,
		`tast.promisify(chrome.autotestPrivate.waitForAmbientPhotoAnimation)`,
		numCompletions,
		toNearestSecond(timeout),
	)
}

// waitForVideo blocks until video playback has started.
func waitForVideo(
	ctx context.Context,
	tconn *chrome.TestConn,
	timeout time.Duration,
) error {
	return tconn.Call(
		ctx,
		nil,
		`tast.promisify(chrome.autotestPrivate.waitForAmbientVideo)`,
		toNearestSecond(timeout),
	)
}

// TestLockScreenIdle performs screen saver test including locking screen, waiting for ambient mode
// to start, then escaping from ambient mode and returning to lockscreen again.
func TestLockScreenIdle(
	cr *chrome.Chrome,
	tconn *chrome.TestConn,
	ui *uiauto.Context,
	testParams TestParams,
) uiauto.Action {
	return func(ctx context.Context) error {
		return uiauto.Combine("start, hide, and restart screen saver",
			lockScreen(ctx, tconn),
			waitForAmbientStart(tconn, ui, testParams),
			hideAmbientMode(tconn, ui, mouseMove),
			waitForAmbientStart(tconn, ui, testParams),
			hideAmbientMode(tconn, ui, mouseClick),
			waitForAmbientStart(tconn, ui, testParams),
			hideAmbientMode(tconn, ui, keyboardClick),
			waitForAmbientStart(tconn, ui, testParams),
		)(ctx)
	}
}

// CloseScreenSaverPreview returns an action to close screen saver preview by clicking left mouse button.
func CloseScreenSaverPreview(tconn *chrome.TestConn, ui *uiauto.Context) uiauto.Action {
	return func(ctx context.Context) error {
		container := nodewith.ClassName("InSessionAmbientModeContainer").Role(role.Window)
		if err := ui.Exists(container)(ctx); err != nil {
			return errors.Wrap(err, "failed to find ambient mode container")
		}

		if err := mouse.Click(tconn, coords.Point{X: 0, Y: 0}, mouse.LeftButton)(ctx); err != nil {
			return errors.Wrap(err, "failed to click mouse")
		}

		// Ambient mode container should not exist.
		if err := ui.WaitUntilGone(container)(ctx); err != nil {
			return errors.Wrap(err, "failed to ensure ambient container dismissed")
		}

		if st, err := lockscreen.GetState(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to get lockscreen state")
		} else if st.Locked {
			return errors.Wrap(err, "failed to ensure that screen is not locked")
		}

		return nil
	}
}

// lockScreen returns an action to lock screen.
func lockScreen(ctx context.Context, tconn *chrome.TestConn) uiauto.Action {
	return func(ctx context.Context) error {
		if err := lockscreen.Lock(ctx, tconn); err != nil {
			errors.Wrap(err, "failed to lock the screen")
		}
		if st, err := lockscreen.WaitState(ctx, tconn, func(st lockscreen.State) bool { return st.Locked && st.ReadyForPassword }, 30*time.Second); err != nil {
			errors.Errorf("failed to wait for screen to be locked: %v (last status %+v)", err, st)
		}
		return nil
	}
}

// UnlockScreen returns an action to  enter the password and unlock screen.
func UnlockScreen(tconn *chrome.TestConn, username, password string) uiauto.Action {
	return func(ctx context.Context) error {
		// Open a keyboard device.
		kb, err := input.Keyboard(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to open keyboard device")
		}
		defer kb.Close(ctx)

		if err := lockscreen.EnterPassword(ctx, tconn, username, password, kb); err != nil {
			return errors.Wrap(err, "failed to unlock the screen")
		}
		if st, err := lockscreen.WaitState(ctx, tconn, func(st lockscreen.State) bool { return !st.Locked }, 30*time.Second); err != nil {
			return errors.Errorf("failed to wait for screen to be unlocked: %v (last status %+v)", err, st)
		}
		return nil
	}
}

// waitForAmbientStart returns an action to wait for ambient mode to start and validate
// the number of photo transitions during ambient mode.
func waitForAmbientStart(tconn *chrome.TestConn, ui *uiauto.Context, testParams TestParams) uiauto.Action {
	return func(ctx context.Context) error {
		var err error
		if testParams.Theme == VideoTheme {
			err = waitForVideo(ctx, tconn, testParams.StartupTimeout)
		} else {
			err = waitForPhotoTransitions(ctx, tconn, 2, testParams.StartupTimeout)
		}
		if err != nil {
			return errors.Wrap(err, "failed to wait for ambient autotest API")
		}
		if testParams.PlayTestVideo {
			return showMediaStringInScreenSaver(ctx, ui)
		}
		if err := ui.WaitUntilExists(
			nodewith.ClassName("LockScreenAmbientModeContainer").Role(role.Window),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to wait for ambient mode start")
		}
		return nil
	}
}

// showMediaStringInScreenSaver returns an error if the media string is missing from
// the screen saver.
func showMediaStringInScreenSaver(ctx context.Context, ui *uiauto.Context) error {
	// For the media string condition:
	// .First() is needed because there are actually 2 media string "nodes"
	// present in the UI tree during slideshow mode. The second node is
	// invisible in the background and is a product of how slideshow mode is
	// implemented. Without ".First()"" though, this condition fails saying
	// that here are multiple matching nodes.
	if err := uiauto.Combine("validate ambient screen and media string",
		ui.WaitUntilExists(nodewith.ClassName("LockScreenAmbientModeContainer").Role(role.Window)),
		ui.WaitUntilExists(nodewith.NameContaining(TestVideoSrc.Title).First()),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to validate ambient screen and media string exist")
	}
	return nil
}

// hideAmbientMode returns an action to break from ambient mode and return to lockscreen.
func hideAmbientMode(
	tconn *chrome.TestConn,
	ui *uiauto.Context,
	hideMethod func(context.Context, *uiauto.Context) error,
) uiauto.Action {
	return func(ctx context.Context) error {
		container := nodewith.ClassName("LockScreenAmbientModeContainer").Role(role.Window)
		if err := ui.Exists(container)(ctx); err != nil {
			return errors.Wrap(err, "failed to find lock screen ambient mode container")
		}

		if err := hideMethod(ctx, ui); err != nil {
			return err
		}

		// Ambient mode container should not exist.
		if err := ui.WaitUntilGone(container)(ctx); err != nil {
			return errors.Wrap(err, "failed to ensure ambient container dismissed")
		}

		if st, err := lockscreen.WaitState(ctx, tconn, func(st lockscreen.State) bool { return st.Locked && st.ReadyForPassword }, 30*time.Second); err != nil {
			errors.Errorf("failed to wait for screen to be locked: %v (last status %+v)", err, st)
		}

		return nil
	}
}

// mouseMove moves the mouse a small amount. Ambient mode should turn off. Session
// should still be locked.
func mouseMove(ctx context.Context, ui *uiauto.Context) error {
	mouse, err := input.Mouse(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get mouse")
	}
	defer mouse.Close(ctx)

	if err := mouse.Move(10, 10); err != nil {
		return errors.Wrap(err, "failed to move mouse")
	}

	if err := mouse.Move(-10, -10); err != nil {
		return errors.Wrap(err, "failed to move mouse")
	}

	return nil
}

// mouseClick triggers a mouse click event to exit ambient mode.
func mouseClick(ctx context.Context, ui *uiauto.Context) error {
	if err := ui.MouseClickAtLocation(0, coords.Point{X: 200, Y: 200})(ctx); err != nil {
		return err
	}
	return nil
}

// keyboardClick triggers a keyboard click event to exit ambient mode.
func keyboardClick(ctx context.Context, ui *uiauto.Context) error {
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get keyboard")
	}
	defer kb.Close(ctx)

	if err := kb.TypeAction("a")(ctx); err != nil {
		return err
	}
	return nil
}

// IsAlbumSelected returns whether the `albumInfo` provided has been selected in
// the hub.
func IsAlbumSelected(albumInfo *uiauto.NodeInfo) bool {
	return strings.Contains(albumInfo.ClassName, AlbumSelectedClassName)
}

// FindAlbumWithName returns the album in `allAlbums` whose name matches
// `albumName`. Returns nil if an album with matching name is not found.
func FindAlbumWithName(albumName string, allAlbums []uiauto.NodeInfo) *uiauto.NodeInfo {
	for _, album := range allAlbums {
		if album.Name == albumName {
			return &album
		}
	}
	return nil
}

// SelectAlbum selects the album with the given `albumInfo` in the hub.
func SelectAlbum(ctx context.Context, ui *uiauto.Context, albumInfo *uiauto.NodeInfo) error {
	selectedAlbumNode := nodewith.HasClass(AlbumSelectedClassName).Name(albumInfo.Name)
	return uiauto.Retry(3, uiauto.Combine("select album",
		ui.Gone(selectedAlbumNode),
		ui.MouseClickAtLocation(0, albumInfo.Location.CenterPoint()),
		ui.WithTimeout(3*time.Second).WaitUntilExists(selectedAlbumNode),
	))(ctx)
}

// DeselectAlbum deselects the album with the given `albumInfo` in the hub.
func DeselectAlbum(ctx context.Context, ui *uiauto.Context, albumInfo *uiauto.NodeInfo) error {
	selectedAlbumNode := nodewith.HasClass(AlbumSelectedClassName).Name(albumInfo.Name)
	return uiauto.Retry(3, uiauto.Combine("deselect album",
		ui.Exists(selectedAlbumNode),
		ui.MouseClickAtLocation(0, albumInfo.Location.CenterPoint()),
		ui.WithTimeout(3*time.Second).WaitUntilGone(selectedAlbumNode),
	))(ctx)
}

// SetScreenSaverHelper runs the entire core screen saver test sequence. It's
// here as a helper function so that multiple test bundles can reuse it.
func SetScreenSaverHelper(ctx context.Context, cr *chrome.Chrome, testParams TestParams, errorOutputDir string, hasError func() bool) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}

	defer faillog.DumpUITreeOnError(cleanupCtx, errorOutputDir, hasError, tconn)

	// The test has a dependency of network speed, so we give uiauto.Context ample
	// time to wait for nodes to load.
	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)

	// Some expectations below test the default ambient settings in the hub, so
	// the pref must be cleared to simulate a user just completing OOBE.
	if err := tconn.Call(ctx, nil, `tast.promisify(chrome.autotestPrivate.clearAllowedPref)`, "ash.ambient.ui_settings"); err != nil {
		return errors.Wrap(err, "failed to reset ambient prefs to default values")
	}

	if err := uiauto.Combine("Open ambient subpage and enable screen saver",
		OpenAmbientSubpage(ui),
		EnableAmbientMode(ui),
		prepareScreenSaver(tconn, ui, testParams))(ctx); err != nil {
		return errors.Wrapf(err, "failed to prepare %v/%v screen saver", testParams.TopicSource, testParams.Theme)
	}

	if testParams.PlayTestVideo {
		kb, err := input.VirtualKeyboard(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to open the keyboard")
		}
		defer kb.Close(cleanupCtx)

		var uiHandler cuj.UIActionHandler
		if uiHandler, err = cuj.NewClamshellActionHandler(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to create clamshell action handler")
		}
		defer uiHandler.Close(cleanupCtx)

		// Open up an arbitrary Youtube video to test "media string". The name of
		// the media playing should be displayed in the screen saver.
		const extendedDisplay = false
		videoApp := youtube.NewYtWeb(cr.Browser(), tconn, kb, extendedDisplay, ui, uiHandler)
		if err := videoApp.OpenAndPlayVideo(TestVideoSrc)(ctx); err != nil {
			return errors.Wrapf(err, "failed to open %s", TestVideoSrc.URL)
		}
		defer videoApp.Close(cleanupCtx)
	}

	if err := uiauto.Combine("Run screen saver and unlock screen",
		TestLockScreenIdle(cr, tconn, ui, testParams),
		UnlockScreen(tconn, cr.Creds().User, cr.Creds().Pass))(ctx); err != nil {
		return errors.Wrapf(err, "failed to run %v/%v screen saver", testParams.TopicSource, testParams.Theme)
	}

	return nil
}

func prepareScreenSaver(tconn *chrome.TestConn, ui *uiauto.Context, testParams TestParams) uiauto.Action {
	return func(ctx context.Context) error {
		themeContainer := nodewith.Role(role.RadioButton).Name(testParams.Theme)
		if err := uiauto.Combine("Choose animation theme",
			ui.FocusAndWait(themeContainer),
			ui.LeftClick(themeContainer))(ctx); err != nil {
			return errors.Wrapf(err, "failed to select %v", testParams.Theme)
		}

		topicSourceContainer := nodewith.Role(role.RadioButton).NameContaining(testParams.TopicSource).Ancestor(nodewith.Attribute("description", "Image source"))
		albumsFinder := nodewith.Role(role.ListBoxOption).HasClass("album")

		if err := uiauto.Combine("Choose topic source",
			ui.FocusAndWait(topicSourceContainer),
			ui.LeftClick(topicSourceContainer),
			ui.WaitUntilExists(albumsFinder.First()))(ctx); err != nil {
			return errors.Wrapf(err, "failed to select %v", testParams.TopicSource)
		}

		albums, err := ui.NodesInfo(ctx, albumsFinder)
		if err != nil {
			return errors.Wrapf(err, "failed to find %v albums", testParams.TopicSource)
		}
		if len(albums) < 2 {
			return errors.Errorf("at least 2 %v albums expected", testParams.TopicSource)
		}

		// For animated themes, trust the default album selection. Test cases for
		// slideshow theme will verify that the default album selection is correct
		// and test custom album selection.
		if testParams.Theme == SlideShow {
			if testParams.TopicSource == GooglePhotos {
				var albumNames []string
				// Select all Google Photos albums.
				for i, album := range albums {
					if IsAlbumSelected(&album) {
						return errors.Errorf("Google Photos album %d should be unselected", i)
					}
					if err := SelectAlbum(ctx, ui, &album); err != nil {
						return errors.Wrapf(err, "failed to select Google Photos album %d", i)
					}
					albumNames = append(albumNames, album.Name)
				}
				expectedText := strings.Join(albumNames, " ")
				if err := waitForCurrentlySetWithName(ui, expectedText)(ctx); err != nil {
					return errors.Wrap(err, "failed to find matching currently set header")
				}
			} else if testParams.TopicSource == ArtGallery {
				// Turn off all but one art gallery album.
				for i, album := range albums[1:] {
					if !IsAlbumSelected(&album) {
						return errors.Errorf("Art album %d should be selected", i)
					}
					if err := DeselectAlbum(ctx, ui, &album); err != nil {
						return errors.Wrapf(err, "failed to deselect Art Gallery album %d", i)
					}
				}
				if err := waitForCurrentlySetWithName(ui, albums[0].Name)(ctx); err != nil {
					return errors.Wrap(err, "failed to wait for currently set")
				}
			} else {
				return errors.Errorf("topicSource - %v is invalid", testParams.TopicSource)
			}
		} else if testParams.Theme == VideoTheme {
			// Always make sure only the default video is selected initially.
			selectedVideos, err := ui.NodesInfo(ctx, nodewith.HasClass(AlbumSelectedClassName))
			if err != nil {
				return errors.Wrap(err, "failed to find the selected video albums")
			}
			if len(selectedVideos) != 1 {
				return errors.New("exactly 1 selected video album expected")
			}
			if selectedVideos[0].Name != DefaultVideoName {
				return errors.New("incorrect default ambient video is selected")
			}

			// Select the correct video if a non-default is requested.
			if testParams.VideoThemeAlbum != DefaultVideoName {
				albumToSelect := FindAlbumWithName(testParams.VideoThemeAlbum, albums)
				if albumToSelect == nil {
					return errors.New("failed to find video album with name " + testParams.VideoThemeAlbum)
				}
				if err := SelectAlbum(ctx, ui, albumToSelect); err != nil {
					return errors.Wrap(err, "failed to select video album "+testParams.VideoThemeAlbum)
				}
			}
		}

		// Close Personalization Hub after ambient mode setup is finished.
		if err := uiauto.Combine("Close personalization app and set device settings",
			personalization.ClosePersonalizationHub(ui),
			SetDeviceSettings(tconn, DeviceSettings{
				LockScreenIdle:         1 * time.Second,
				BackgroundLockScreen:   2 * time.Second,
				PhotoRefreshInterval:   1 * time.Second,
				AnimationPlaybackSpeed: testParams.AnimationPlaybackSpeed,
			}))(ctx); err != nil {
			return errors.Wrap(err, "failed to prepare screen saver")
		}

		return nil
	}
}

func waitForCurrentlySetWithName(ui *uiauto.Context, name string) uiauto.Action {
	currentlySetNode := nodewith.NameStartingWith(fmt.Sprintf("Currently set %v", name)).Role(role.Alert).Ancestor(personalization.PersonalizationHubWindow)
	return ui.WaitUntilExists(currentlySetNode)
}
