// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package a11y provides functions to assist with interacting with accessibility
// features and settings.
package a11y

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/a11y/tts"
	"go.chromium.org/tast-tests/cros/local/audio/crastests"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
)

// List of extension IDs and URLs.
const (
	AccessibilityCommonExtensionURL = "chrome-extension://egfdjlfmgnehecnclamagfafdccgfndp/accessibility_common/accessibility_common_loader.js"
	ChromeVoxExtensionURL           = "chrome-extension://mndnfokpggljbaajbnioimlmbfngpief/chromevox/mv2/background/background.html"
	SelectToSpeakExtensionURL       = "chrome-extension://klbcgckkldhdhonijdbnhhaiedfkllef/select_to_speak/mv2/background.html"
)

// Feature represents an accessibility feature in ChromeOS.
type Feature string

// List of accessibility features.
const (
	Autoclick       Feature = "autoclick"
	Dictation       Feature = "dictation"
	DockedMagnifier Feature = "dockedMagnifier"
	FocusHighlight  Feature = "focusHighlight"
	ScreenMagnifier Feature = "screenMagnifier"
	SelectToSpeak   Feature = "selectToSpeak"
	SpokenFeedback  Feature = "spokenFeedback"
	SwitchAccess    Feature = "switchAccess"
)

// ReadOnlyGoogleDocURL is a URL to a read-only Google doc that contains only the text,
// "Long-string-to-test-select-to-speak".
const ReadOnlyGoogleDocURL string = "https://docs.google.com/document/d/1qpu3koSIHpBzQbxeEE-dofSKXCIgdc4yJLI-o1LpCPs/view"

const (
	// BounceKeysDefaultDelay is the default delay of Bounce Keys.
	BounceKeysDefaultDelay = 500 * time.Millisecond
	// SlowKeysDefaultDelay is the default delay of Slow Keys.
	SlowKeysDefaultDelay = 500 * time.Millisecond
	// MouseKeysDefaultDelay dictates cursor movement granularity
	MouseKeysDefaultDelay = 100 * time.Millisecond
)

// MouseAction represents the mouse key actions.
type MouseAction int

// Mouse key actions.
const (
	MouseActionMoveUp MouseAction = iota
	MouseActionMoveDown
	MouseActionMoveLeft
	MouseActionMoveRight
	MouseActionClick
	MouseActionSwitchMouseButton
)

// MouseButton represents the different mouse buttons.
type MouseButton int

// Mouse key buttons.
const (
	LeftMouseButton MouseButton = iota
	RightMouseButton
	BothMouseButtons
)

// MouseActionToKeyboardKey maps MouseAction to corresponding keyboard keys.
func MouseActionToKeyboardKey(button MouseAction) string {
	switch button {
	case MouseActionMoveUp:
		return "8"
	case MouseActionMoveDown:
		return "k"
	case MouseActionMoveLeft:
		return "u"
	case MouseActionMoveRight:
		return "o"
	case MouseActionClick:
		return "i"
	case MouseActionSwitchMouseButton:
		return ","
	default:
		return ""
	}
}

// ChangeMouseButton cycles the current mouse button to the target button.
func ChangeMouseButton(ctx context.Context, kb *input.KeyboardEventWriter, current, target MouseButton) (MouseButton, error) {
	for current != target {
		if err := kb.Type(ctx, MouseActionToKeyboardKey(MouseActionSwitchMouseButton)); err != nil {
			return current, err
		}

		if current == LeftMouseButton {
			current = RightMouseButton
		} else if current == RightMouseButton {
			current = BothMouseButtons
		} else if current == BothMouseButtons {
			current = LeftMouseButton
		}
	}
	return current, nil
}

// SetFaceGazeEnabled enables the FaceGaze accessibility feature using the
// settings private extension API. The reason we need this is that FaceGaze is
// still in development and hasn't been added to the accessibility features API,
// which is used below to control other accessibility features.
func SetFaceGazeEnabled(ctx context.Context, tconn *chrome.TestConn, enabled bool) error {
	return tconn.Call(ctx, nil, "tast.promisify(chrome.settingsPrivate.setPref)", "settings.a11y.face_gaze.enabled", enabled)
}

// SetFeatureEnabled forcibly enables/disables the specified accessibility
// feature using the provided connection to the extension.
// NOTE: This can have the side effect of disabling UI elements such as toggles
// through which the user can normally control the feature.
// NOTE: This should be used together with a deferred call to ClearFeature.
func SetFeatureEnabled(ctx context.Context, tconn *chrome.TestConn, feature Feature, enable bool) error {
	if err := tconn.Call(ctx, nil, `(feature, enable) => {
      return tast.promisify(tast.bind(chrome.accessibilityFeatures[feature], "set"))({value: enable});
    }`, feature, enable); err != nil {
		return errors.Wrapf(err, "failed to toggle %v to %t", feature, enable)
	}
	return nil
}

// ClearFeature effectively undoes previous SetFeatureEnabled calls for the given feature.
func ClearFeature(ctx context.Context, tconn *chrome.TestConn, feature Feature) error {
	if err := tconn.Call(ctx, nil, `(feature) => {
      return tast.promisify(tast.bind(chrome.accessibilityFeatures[feature], "clear"))({});
    }`, feature); err != nil {
		return errors.Wrapf(err, "failed to clear %v", feature)
	}
	return nil
}

// FeatureEnabled gets the current enabled state of the given feature.
func FeatureEnabled(ctx context.Context, tconn *chrome.TestConn, feature Feature) (bool, error) {
	var enabled bool
	if err := tconn.Eval(ctx, fmt.Sprintf(`(async() => {
			let state = await tast.promisify(tast.bind(chrome.accessibilityFeatures['%s'], 'get'))({});
			return state.value;
		})()`, feature), &enabled); err != nil {
		return false, err
	}
	return enabled, nil
}

// URLFromHTML converts html in to a data:/text/html URL.
func URLFromHTML(html string) string {
	return fmt.Sprintf("data:text/html, %s", html)
}

// NewTabWithURL creates a new tab with the specified URL, waits for it to
// load, and returns a connection to the page.
func NewTabWithURL(ctx context.Context, cr *chrome.Chrome, url string) (*chrome.Conn, error) {
	c, err := cr.NewConn(ctx, url, browser.WithNewWindow())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open new tab with url: %s", url)
	}

	if err := c.WaitForExpr(ctx, `document.readyState === "complete"`); err != nil {
		c.Close()
		return nil, errors.Wrap(err, "timed out waiting for page to load")
	}

	return c, nil
}

// VerifyFaceGazeAssetsInstalled checks if the facegaze-assets dlc is installed.
func VerifyFaceGazeAssetsInstalled(ctxt context.Context) error {
	const facegazeMount = "/run/imageloader/facegaze-assets/package/root"
	if _, err := os.Stat(facegazeMount); err != nil {
		return errors.Wrap(err, "dlc facegaze-assets is not installed")
	}

	return nil
}

// VerifySodaInstalled checks if dlc libsoda and libsoda-model-en-us are installed.
func VerifySodaInstalled(ctx context.Context) error {
	const templateMnt = "/run/imageloader/%s/package/root"

	// TODO(b/261775478): Figure out why "--list" flakes.
	for _, id := range []string{"libsoda", "libsoda-model-en-us*"} {
		mnt := fmt.Sprintf(templateMnt, id)
		if matches, err := filepath.Glob(mnt); err != nil || matches == nil {
			errStr := fmt.Sprintf("dlc %s is not installed", id)
			return errors.Wrap(err, errStr)
		}
	}

	return nil
}

// VerifyScreenAIInstalled checks if the screen-ai dlc is installed.
func VerifyScreenAIInstalled(ctx context.Context) error {
	const screenAIMnt = "/run/imageloader/screen-ai/package/root"
	if _, err := os.Stat(screenAIMnt); err != nil {
		return errors.Wrap(err, "dlc screen-ai is not installed")
	}

	return nil
}

// TTSFeatureInputs represents data used for setting up an accessibility
// feature that uses TTS e.g. ChromeVox or Select-to-Speak. URL specifies
// the URL to run the test on.
type TTSFeatureInputs struct {
	CTX     context.Context
	CR      *chrome.Chrome
	ED      tts.EngineData
	URL     string
	Feature Feature
}

// TTSFeatureData contains data and useful objects for an accessibility feature
// that uses TTS. Tconn and SM live until TDown.TearDown() is called.
type TTSFeatureData struct {
	CTX   context.Context
	TConn *chrome.TestConn
	SM    *tts.SpeechMonitor
	TDown *TearDownHelper
	Conn  *chrome.Conn
}

func newNoOpTTSFeatureData(tdh *TearDownHelper) TTSFeatureData {
	return TTSFeatureData{TDown: tdh}
}

// TearDownHelper represents cleanup functions that should be run in a
// defer statement by the calling test.
type TearDownHelper struct {
	funcs []func() error
}

// TearDown iterates backwards through cleanUpFuncs, since cleanUpFuncs represents
// deferred methods. It also removes functions once executed to ensure they
// don't get run more than once.
func (tdh *TearDownHelper) TearDown() error {
	var errs []error
	for index := len(tdh.funcs) - 1; index >= 0; index-- {
		step := tdh.funcs[index]
		if err := step(); err != nil {
			errs = append(errs, err)
		}
		tdh.funcs = tdh.funcs[:index]
	}

	if len(errs) > 0 {
		return errors.Errorf("failed tear down steps: %q", errs)
	}

	return nil
}

// Append pushes a function to be run at tear down.
func (tdh *TearDownHelper) Append(f func() error) {
	tdh.funcs = append(tdh.funcs, f)
}

// SetUpTTSFeature runs common setup code needed for features that require
// text-to-speech. This includes ChromeVox and Select-to-Speak. This function
// does several things including:
// 1. Muting the device
// 2. Setting up a browser and loading HTML
// 3. Turning on the feature
// 4. Connecting to a TTS engine
// 5. Populating cleanup functions
func SetUpTTSFeature(tfi TTSFeatureInputs) (tfd TTSFeatureData, e error) {
	defer func() {
		if e != nil {
			tfd.TDown.TearDown()
		}
	}()

	// Extract inputs.
	ctx := tfi.CTX
	cr := tfi.CR
	ed := tfi.ED
	url := tfi.URL
	feature := tfi.Feature

	tdown := &TearDownHelper{}

	// Shorten deadline to leave time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	tdown.Append(func() error {
		cancel()
		return nil
	})

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return newNoOpTTSFeatureData(tdown), errors.Wrap(err, "failed to create Test API connection")
	}

	// Mute the device to avoid noisiness.
	if err := crastests.Mute(ctx); err != nil {
		return newNoOpTTSFeatureData(tdown), errors.Wrap(err, "failed to mute device")
	}
	tdown.Append(func() error {
		return crastests.Unmute(cleanupCtx)
	})

	if err := SetFeatureEnabled(ctx, tconn, feature, true); err != nil {
		return newNoOpTTSFeatureData(tdown), errors.Wrapf(err, "failed to enable feature: %s", feature)
	}
	tdown.Append(func() error {
		if err := ClearFeature(cleanupCtx, tconn, feature); err != nil {
			return errors.Wrapf(err, "failed to disable feature: %s", feature)
		}

		return nil
	})

	sm, err := tts.RelevantSpeechMonitor(ctx, cr, tconn, ed)
	if err != nil {
		return newNoOpTTSFeatureData(tdown), errors.Wrap(err, "failed to connect to the TTS background page")
	}
	tdown.Append(func() error {
		return sm.Close()
	})

	if err := tts.SetRate(ctx, tconn, 1.0); err != nil {
		return newNoOpTTSFeatureData(tdown), errors.Wrap(err, "failed to change TTS rate")
	}

	crConn, err := NewTabWithURL(ctx, cr, url)
	if err != nil {
		return newNoOpTTSFeatureData(tdown), errors.Wrap(err, "failed to open a new tab with URL")
	}
	tdown.Append(func() error {
		return crConn.Close()
	})

	return TTSFeatureData{ctx, tconn, sm, tdown, crConn}, nil
}

// toggleKeyboardAndTextInputSetting is a helper function that toggles
// a setting in Keyboard and Text Input settings page via Settings UI.
func toggleKeyboardAndTextInputSetting(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome, ui *uiauto.Context, name string, settingId int, enable bool) error {
	heading := nodewith.NameStartingWith("Keyboard and text input").Role(role.Heading).Ancestor(ossettings.WindowFinder)
	kbSettings, err := ossettings.LaunchAtPageURL(ctx, tconn, cr, fmt.Sprintf("keyboardAndTextInput?settingId=%d", settingId), ui.Exists(heading))
	if err != nil {
		return errors.Wrap(err, "failed to open keyboard accessibility settings page")
	}
	defer kbSettings.Close(ctx)
	if err := kbSettings.SetToggleOption(cr, name, enable)(ctx); err != nil {
		return errors.Wrapf(err, "failed to toggle %q setting", name)
	}
	return nil
}

// ToggleBounceKeysSetting is a helper function that toggles Bounce Keys setting via Settings UI.
func ToggleBounceKeysSetting(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome, ui *uiauto.Context, enable bool) error {
	return toggleKeyboardAndTextInputSetting(ctx, tconn, cr, ui, "Bounce keys", 1554, enable)
}

// ToggleSlowKeysSetting is a helper function that toggles Slow Keys setting via Settings UI.
func ToggleSlowKeysSetting(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome, ui *uiauto.Context, enable bool) error {
	return toggleKeyboardAndTextInputSetting(ctx, tconn, cr, ui, "Slow keys", 1555, enable)
}

func toggleCursorAndTouchpadSetting(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome, ui *uiauto.Context, name string, settingId int, enable bool) error {
	heading := nodewith.NameStartingWith("Cursor and touchpad").Role(role.Heading).Ancestor(ossettings.WindowFinder)
	cursorTouchpadSettings, err := ossettings.LaunchAtPageURL(ctx, tconn, cr, fmt.Sprintf("cursorAndTouchpad?settingId=%d", settingId), ui.Exists(heading))
	if err != nil {
		return errors.Wrap(err, "failed to open cursor and touchpad settings page")
	}
	defer cursorTouchpadSettings.Close(ctx)
	if err := cursorTouchpadSettings.SetToggleOption(cr, name, enable)(ctx); err != nil {
		return errors.Wrapf(err, "failed to toggle %q setting", name)
	}
	return nil
}

// ToggleMouseKeysSetting is a helper function that toggles Mouse Keys setting via Settings UI.
func ToggleMouseKeysSetting(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome, ui *uiauto.Context, enable bool) error {
	return toggleCursorAndTouchpadSetting(ctx, tconn, cr, ui, "Mouse Keys", 1538, enable)
}
