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
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/a11y/tts"
	"chromiumos/tast/local/audio/crastestclient"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/input"
)

// List of extension IDs and URLs.
const (
	ChromeVoxExtensionURL = "chrome-extension://mndnfokpggljbaajbnioimlmbfngpief/chromevox/background/background.html"
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

// NewTabWithHTML creates a new tab with the specified HTML, waits for it to
// load, and returns a connection to the page.
// This works with either ash-chrome or lacros-chrome browser.
func NewTabWithHTML(ctx context.Context, br *browser.Browser, html string) (*browser.Conn, error) {
	url := fmt.Sprintf("data:text/html, %s", html)
	c, err := br.NewConn(ctx, url, browser.WithNewWindow())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open new tab with url: %s", url)
	}

	if err := c.WaitForExpr(ctx, `document.readyState === "complete"`); err != nil {
		c.Close()
		return nil, errors.Wrap(err, "timed out waiting for page to load")
	}

	return c, nil
}

// MaybeCloseDictationDialog closes the dialog that is shown when Dictation is first
// enabled, if it appears on the screen. The dialog warns the user that their voice
// is sent to Google. This function accepts the dialog so we can use the feature.
func MaybeCloseDictationDialog(ctx context.Context, ui *uiauto.Context) error {
	dialogText := nodewith.NameContaining("Dictation sends your voice to Google").Onscreen()
	continueButton := nodewith.Name("Continue").ClassName("MdTextButton").Onscreen()

	// Check if the dialog pops up.
	if err := uiauto.Combine("Find Dictation dialog",
		ui.WaitUntilExists(dialogText),
	)(ctx); err != nil {
		// If the Dictation dialog can't be found, then we don't need to do anything.
		return nil
	}

	if err := uiauto.Combine("Close Dictation dialog",
		ui.LeftClick(continueButton),
		ui.WaitUntilGone(dialogText),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to close the Dictation dialog")
	}

	return nil
}

// ToggleDictation presses Search + D on the keyboard to either turn Dictation
// on or off, depending on the current state.
func ToggleDictation(ctx context.Context) error {
	ew, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create EventWriter")
	}
	defer ew.Close()

	if err := ew.Accel(ctx, "Search+D"); err != nil {
		return errors.Wrap(err, "failed to press Search + D to toggle Dictation")
	}

	return nil
}

// VerifySodaInstalled checks if dlc libsoda and libsoda-model-en-us are installed.
func VerifySodaInstalled(ctx context.Context) error {
	const templateMnt = "/run/imageloader/%s/package/root"

	// TODO(b/261775478): Figure out why "--list" flakes.
	for _, id := range []string{"libsoda", "libsoda-model-en-us"} {
		mnt := fmt.Sprintf(templateMnt, id)
		if _, err := os.Stat(mnt); err != nil {
			errStr := fmt.Sprintf("dlc %s is not installed", id)
			return errors.Wrap(err, errStr)
		}
	}

	return nil
}

// TTSFeatureInputs represents data used for setting up an accessibility
// feature that uses TTS e.g. ChromeVox or Select-to-Speak. HTML specifies the
// web content to load and run a test on.
type TTSFeatureInputs struct {
	CTX     context.Context
	CR      *chrome.Chrome
	ED      tts.EngineData
	BT      browser.Type
	HTML    string
	Feature Feature
}

// TTSFeatureData contains data and useful objects for an accessibility feature
// that uses TTS. Tconn and SM live until TDown.TearDown() is called.
type TTSFeatureData struct {
	CTX   context.Context
	TConn *chrome.TestConn
	SM    *tts.SpeechMonitor
	TDown *TTSFeatureTearDown
}

func newNilTTSFeatureData(tftd *TTSFeatureTearDown) TTSFeatureData {
	return TTSFeatureData{TDown: tftd}
}

// TTSFeatureTearDown represents cleanup functions that should be run in a
// defer statement by the calling test.
type TTSFeatureTearDown struct {
	funcs []func() error
}

// TearDown iterates backwards through cleanUpFuncs, since cleanUpFuncs represents
// deferred methods. It also removes functions once executed to ensure they
// don't get run more than once.
func (tftd *TTSFeatureTearDown) TearDown() error {
	var errs []error
	for index := len(tftd.funcs) - 1; index >= 0; index-- {
		step := tftd.funcs[index]
		if err := step(); err != nil {
			errs = append(errs, err)
		}
		tftd.funcs = tftd.funcs[:index]
	}

	if len(errs) > 0 {
		return errors.Errorf("failed tear down steps: %q", errs)
	}

	return nil
}

// Append pushes a function to be run at tear down.
func (tftd *TTSFeatureTearDown) Append(f func() error) {
	tftd.funcs = append(tftd.funcs, f)
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
	bt := tfi.BT
	html := tfi.HTML
	feature := tfi.Feature

	tdown := &TTSFeatureTearDown{}

	// Shorten deadline to leave time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	tdown.Append(func() error {
		cancel()
		return nil
	})

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return newNilTTSFeatureData(tdown), errors.Wrap(err, "failed to create Test API connection")
	}

	// Mute the device to avoid noisiness.
	if err := crastestclient.Mute(ctx); err != nil {
		return newNilTTSFeatureData(tdown), errors.Wrap(err, "failed to mute device")
	}
	tdown.Append(func() error {
		return crastestclient.Unmute(cleanupCtx)
	})

	// Setup a browser.
	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, bt)
	if err != nil {
		return newNilTTSFeatureData(tdown), errors.Wrap(err, "failed to setup browser")
	}
	tdown.Append(func() error {
		return closeBrowser(cleanupCtx)
	})

	brConn, err := NewTabWithHTML(ctx, br, html)
	if err != nil {
		return newNilTTSFeatureData(tdown), errors.Wrap(err, "failed to open a new tab with HTML")
	}
	tdown.Append(func() error {
		return brConn.Close()
	})

	// Close the extra new tab page.
	if err := br.CloseWithURL(ctx, chrome.NewTabURL); err != nil {
		return newNilTTSFeatureData(tdown), errors.Wrap(err, "failed to close new tab page")
	}

	if err := SetFeatureEnabled(ctx, tconn, feature, true); err != nil {
		return newNilTTSFeatureData(tdown), errors.Wrapf(err, "failed to enable feature: %s", feature)
	}
	tdown.Append(func() error {
		if err := ClearFeature(cleanupCtx, tconn, feature); err != nil {
			return errors.Wrapf(err, "failed to disable feature: %s", feature)
		}

		return nil
	})

	sm, err := tts.RelevantSpeechMonitor(ctx, cr, tconn, ed)
	if err != nil {
		return newNilTTSFeatureData(tdown), errors.Wrap(err, "failed to connect to the TTS background page")
	}
	tdown.Append(func() error {
		return sm.Close()
	})

	if err := tts.SetRate(ctx, tconn, 1.0); err != nil {
		return newNilTTSFeatureData(tdown), errors.Wrap(err, "failed to change TTS rate")
	}

	return TTSFeatureData{ctx, tconn, sm, tdown}, nil
}
