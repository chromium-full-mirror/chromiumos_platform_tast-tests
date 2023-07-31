// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package chromevox provides functions to assist with interacting with ChromeVox, the built in screenreader.
package chromevox

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/a11y/tts"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Constants for keyboard shortcuts.
const (
	Activate         = "Search+Space"
	ArrowDown        = "Down"
	Escape           = "Esc"
	Find             = "Ctrl+F"
	JumpToLauncher   = "Alt+Shift+L"
	JumpToStatusTray = "Alt+Shift+S"
	NextObject       = "Search+Right"
	PreviousObject   = "Search+Left"
	CloseWindow      = "Ctrl+W"
	Space            = "Space"
)

// OpenOptionsPage is the run of keyboard shortcuts used to open the ChromeVox options page.
var OpenOptionsPage = []string{
	"Search+O",
	"O",
}

// VoiceData contains context about voice, language, and TTS engine data to use for a given ChromeVox instance.
type VoiceData struct {
	VoiceData  tts.VoiceData
	EngineData tts.EngineData
}

// Conn represents a connection to the ChromeVox background page.
type Conn struct {
	*chrome.Conn
}

// NewConn returns a connection to the ChromeVox extension's background page.
// If the extension is not ready, the connection will be closed before returning.
// Otherwise the calling function will close the connection.
func NewConn(ctx context.Context, c *chrome.Chrome) (_ *Conn, e error) {
	extConn, err := c.NewConnForTarget(ctx, chrome.MatchTargetURL(a11y.ChromeVoxExtensionURL))
	if err != nil {
		return nil, err
	}

	defer func() {
		if e != nil {
			extConn.Close()
		}
	}()

	// Poll until ChromeVox connection finishes loading.
	if err := extConn.WaitForExpr(ctx, `document.readyState === "complete"`); err != nil {
		return nil, errors.Wrap(err, "timed out waiting for ChromeVox connection to be ready")
	}

	// Make sure required modules exist and are accessible.
	if err := extConn.Eval(ctx, `(async () => {
		if (!window.TtsBackground) {
		  window.TtsBackground = (await import('/chromevox/background/tts_background.js')).TtsBackground;
		}
		if (!window.ChromeVoxRange) {
		  window.ChromeVoxRange = (await import('/chromevox/background/chromevox_range.js')).ChromeVoxRange;
		}
	  })()`, nil); err != nil {
		return nil, errors.Wrap(err, "failed to export modules from ChromeVox")
	}

	if err := extConn.WaitForExpr(ctx, "ChromeVoxRange.instance"); err != nil {
		return nil, errors.Wrap(err, "ChromeVoxRange is unavailable")
	}

	if err := chrome.AddTastLibrary(ctx, extConn); err != nil {
		return nil, errors.Wrap(err, "failed to introduce tast library")
	}

	return &Conn{extConn}, nil
}

// SetUpData contains useful objects for ChromeVox tests and is
// returned by SetUp. Most notably, TTSData contains data and useful objects for
// interacting with TTS. It also contains a tear-down object that should be
// run in a defer statement to properly clean up ChromeVox.
type SetUpData struct {
	CVConn  *Conn
	TTSData a11y.TTSFeatureData
}

// Context is a convenience method for accessing a context.
func (data SetUpData) Context() context.Context {
	return data.TTSData.CTX
}

// SpeechMonitor is a convenience method for accessing a SpeechMonitor.
func (data SetUpData) SpeechMonitor() *tts.SpeechMonitor {
	return data.TTSData.SM
}

// TearDown is a convenience method for tearing down the SetUpData.
func (data SetUpData) TearDown() error {
	return data.TTSData.TDown.TearDown()
}

// SetUp executes common ChromeVox setup code. Returns a SetUpData - see the
// documentation for SetUpData for information on proper cleanup.
func SetUp(ctx context.Context, cr *chrome.Chrome, vd tts.VoiceData, ed tts.EngineData, bt browser.Type, html string) (setUpData SetUpData, e error) {
	// Tears down ChromeVox if SetUp encountered an error.
	defer func() {
		if e != nil {
			setUpData.TearDown()
		}
	}()

	inputs := a11y.TTSFeatureInputs{CTX: ctx, CR: cr, ED: ed, BT: bt, URL: a11y.URLFromHTML(html), Feature: a11y.SpokenFeedback}
	sud, err := setUpHelper(inputs, vd)

	// Wait for ChromeVox to focus the root web area.
	rootWebArea := nodewith.Role(role.RootWebArea).First()
	if err = sud.CVConn.WaitForFocusedNode(ctx, sud.TTSData.TConn, rootWebArea); err != nil {
		return SetUpData{nil, sud.TTSData}, errors.Wrap(err, "failed to wait for initial ChromeVox focus")
	}

	return sud, nil
}

// SetUpWithURLWithoutFocusWaiter executes common ChromeVox setup code and loads a URL, but
// does not wait for initial focus as that depends on the URL (focus might not always be
// the root web area if something within the loaded HTML requests focus). Returns a
// SetUpData - see the documentation for SetUpData for information on proper cleanup.
func SetUpWithURLWithoutFocusWaiter(ctx context.Context, cr *chrome.Chrome, vd tts.VoiceData, ed tts.EngineData, bt browser.Type, url string) (setUpData SetUpData, e error) {
	inputs := a11y.TTSFeatureInputs{CTX: ctx, CR: cr, ED: ed, BT: bt, URL: url, Feature: a11y.SpokenFeedback}
	return setUpHelper(inputs, vd)
}

// setUpHelper is a helper to execute common ChromeVox setup code.
func setUpHelper(inputs a11y.TTSFeatureInputs, vd tts.VoiceData) (setUpData SetUpData, e error) {
	// Tears down ChromeVox if setUpHelper encountered an error.
	defer func() {
		if e != nil {
			setUpData.TearDown()
		}
	}()

	ttsData, err := a11y.SetUpTTSFeature(inputs)
	if err != nil {
		return SetUpData{nil, ttsData}, errors.Wrap(err, "failed to setup common TTS feature state")
	}

	cvconn, err := NewConn(inputs.CTX, inputs.CR)
	if err != nil {
		return SetUpData{nil, ttsData}, errors.Wrap(err, "failed to connect to the ChromeVox background page")
	}
	ttsData.TDown.Append(func() error {
		cvconn.Close()
		return nil
	})

	if err := cvconn.SetVoice(inputs.CTX, vd); err != nil {
		return SetUpData{nil, ttsData}, errors.Wrap(err, "failed to set the ChromeVox voice")
	}

	return SetUpData{cvconn, ttsData}, nil
}

// focusedNode returns the currently focused node of ChromeVox.
func (cv *Conn) focusedNode(ctx context.Context) (*uiauto.NodeInfo, error) {
	rangeIsValid := false
	if err := cv.Eval(ctx, "!!ChromeVoxRange.current", &rangeIsValid); err != nil {
		return nil, errors.Wrap(err, "failed to check for current range")
	}

	if !rangeIsValid {
		return nil, nil
	}

	var info uiauto.NodeInfo
	script := fmt.Sprintf(`(() => {
		const node = ChromeVoxRange.current.start.node;
		return %s;
	})()`, uiauto.NodeInfoJS)

	if err := cv.Eval(ctx, script, &info); err != nil {
		return nil, errors.Wrap(err, "failed to retrieve the currently focused ChromeVox node")
	}
	return &info, nil
}

// WaitForFocusedNode polls until the properties of the focused node matches the finder.
// timeout specifies the timeout to use when polling.
func (cv *Conn) WaitForFocusedNode(ctx context.Context, tconn *chrome.TestConn, finder *nodewith.Finder) error {
	ui := uiauto.New(tconn)

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		focused, err := cv.focusedNode(ctx)
		if err != nil {
			return testing.PollBreak(err)
		}

		if focused == nil {
			return errors.New("no current ChromeVox focus")
		}

		if match, err := ui.Matches(ctx, finder, focused); err != nil {
			return testing.PollBreak(err)
		} else if !match {
			return errors.Errorf("focused node is incorrect: got %v, want %s", focused, finder.Pretty())
		}
		return nil
	}, &testing.PollOptions{Timeout: 15 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to get current focus")
	}
	return nil
}

// SetVoice sets the ChromeVox's voice, which is specified by using an extension
// ID and a locale.
func (cv *Conn) SetVoice(ctx context.Context, vd tts.VoiceData) error {
	voices, err := tts.Voices(ctx, cv.Conn)
	if err != nil {
		return errors.Wrap(err, "failed to getVoices")
	}
	for _, voice := range voices {
		if voice.ExtID == vd.ExtID && voice.Locale == vd.Locale {
			expr := fmt.Sprintf(`chrome.settingsPrivate.setPref('settings.a11y.chromevox.voice_name', %q);`, voice.Name)
			if err := cv.Eval(ctx, expr, nil); err != nil {
				return err
			}

			// Wait for ChromeVox's current voice to update.
			if err := testing.Poll(ctx, func(ctx context.Context) error {
				var actualVoicename string
				if err := cv.Eval(ctx, "TtsBackground.primary.currentVoice", &actualVoicename); err != nil {
					return err
				}

				if actualVoicename != voice.Name {
					return errors.New("ChromeVox's voice has not yet been updated yet")
				}

				return nil
			}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
				return errors.Wrapf(err, "failed to wait for ChromeVox to update its current voice to: %s", voice.Name)
			}

			return nil
		}
	}

	return errors.Errorf("could not find voice with extension ID: %s and locale: %s", vd.ExtID, vd.Locale)
}
