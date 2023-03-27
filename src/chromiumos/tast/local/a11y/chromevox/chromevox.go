// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package chromevox provides functions to assist with interacting with ChromeVox, the built in screenreader.
package chromevox

import (
	"context"
	"fmt"
	"time"

	"chromiumos/tast/errors"
	"chromiumos/tast/local/a11y"
	"chromiumos/tast/local/audio/crastestclient"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/testing"
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
	VoiceData  a11y.VoiceData
	EngineData a11y.TTSEngineData
}

// Conn represents a connection to the ChromeVox background page.
type Conn struct {
	*chrome.Conn
}

// NewConn returns a connection to the ChromeVox extension's background page.
// If the extension is not ready, the connection will be closed before returning.
// Otherwise the calling function will close the connection.
func NewConn(ctx context.Context, c *chrome.Chrome) (*Conn, error) {
	extConn, err := c.NewConnForTarget(ctx, chrome.MatchTargetURL(a11y.ChromeVoxExtensionURL))
	if err != nil {
		return nil, err
	}

	if err := func() error {
		// Poll until ChromeVox connection finishes loading.
		if err := extConn.WaitForExpr(ctx, `document.readyState === "complete"`); err != nil {
			return errors.Wrap(err, "timed out waiting for ChromeVox connection to be ready")
		}

		// Make sure required modules exist and are accessible.
		if err := extConn.Eval(ctx, `(async () => {
			if (!window.ChromeVoxState) {
			  window.ChromeVoxState = (await import('/chromevox/background/chromevox_state.js')).ChromeVoxState;
			}
			if (!window.TtsBackground) {
			  window.TtsBackground = (await import('/chromevox/background/tts_background.js')).TtsBackground;
			}
			if (!window.ChromeVoxRange) {
			  window.ChromeVoxRange = (await import('/chromevox/background/chromevox_range.js')).ChromeVoxRange;
			}
		  })()`, nil); err != nil {
			return errors.Wrap(err, "failed to export modules from ChromeVox")
		}
		if err := extConn.WaitForExpr(ctx, "ChromeVoxState.instance && ChromeVoxRange.instance"); err != nil {
			return errors.Wrap(err, "ChromeVoxState or ChromeVoxRange is unavailable")
		}

		if err := chrome.AddTastLibrary(ctx, extConn); err != nil {
			return errors.Wrap(err, "failed to introduce tast library")
		}
		return nil
	}(); err != nil {
		extConn.Close()
		return nil, err
	}

	return &Conn{extConn}, nil
}

// SetUpData contains useful objects for ChromeVox tests and is
// returned by SetUp. Most notably, TearDown is a function that should
// be run in a defer statement by the caller to properly tear-down ChromeVox.
// CVConn and SM will remain alive until TearDown is called.
type SetUpData struct {
	CVConn   *Conn
	SM       *a11y.SpeechMonitor
	TearDown func() error
}

// SetUp executes common ChromeVox setup code. Returns a
// SetUpData. When the error is nil, SetUpData will contain a
// non-nil TearDown function. The caller should call it in a defer statement
// for proper cleanup.
func SetUp(ctx, cleanupCtx context.Context, cr *chrome.Chrome, vd a11y.VoiceData, ed a11y.TTSEngineData, bt browser.Type, html string) (_ SetUpData, e error) {
	var cleanUpFuncs []func() error
	tearDown := func() error {
		var errs []error
		// Iterate backwards over cleanUpFuncs, since these are deferred methods.
		for i := len(cleanUpFuncs) - 1; i >= 0; i-- {
			step := cleanUpFuncs[i]
			if err := step(); err != nil {
				errs = append(errs, err)
			}
		}

		if len(errs) > 0 {
			return errors.Errorf("failed ChromeVox tear down steps: %q", errs)
		}

		return nil
	}

	// Tears down ChromeVox if SetUp encountered an error.
	defer func() {
		if e != nil {
			tearDown()
		}
	}()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return SetUpData{}, errors.Wrap(err, "failed to create Test API connection")
	}

	// Mute the device to avoid noisiness.
	if err := crastestclient.Mute(ctx); err != nil {
		return SetUpData{}, errors.Wrap(err, "failed to mute device")
	}
	cleanUpFuncs = append(cleanUpFuncs, func() error {
		crastestclient.Unmute(cleanupCtx)
		return nil
	})

	// Setup a browser.
	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, bt)
	if err != nil {
		return SetUpData{}, errors.Wrap(err, "failed to setup browser")
	}
	cleanUpFuncs = append(cleanUpFuncs, func() error {
		closeBrowser(ctx)
		return nil
	})

	brConn, err := a11y.NewTabWithHTML(ctx, br, html)
	if err != nil {
		return SetUpData{}, errors.Wrap(err, "failed to open a new tab with HTML")
	}
	cleanUpFuncs = append(cleanUpFuncs, func() error {
		brConn.Close()
		return nil
	})

	// Close the extra new tab page.
	if err := br.CloseWithURL(ctx, chrome.NewTabURL); err != nil {
		return SetUpData{}, errors.Wrap(err, "failed to close new tab page")
	}

	if err := a11y.SetFeatureEnabled(ctx, tconn, a11y.SpokenFeedback, true); err != nil {
		return SetUpData{}, errors.Wrap(err, "failed to enable ChromeVox")
	}
	cleanUpFuncs = append(cleanUpFuncs, func() error {
		if err := a11y.ClearFeature(ctx, tconn, a11y.SpokenFeedback); err != nil {
			return errors.Wrap(err, "failed to disable ChromeVox")
		}

		return nil
	})

	cvconn, err := NewConn(ctx, cr)
	if err != nil {
		return SetUpData{}, errors.Wrap(err, "failed to connect to the ChromeVox background page")
	}
	cleanUpFuncs = append(cleanUpFuncs, func() error {
		cvconn.Close()
		return nil
	})

	sm, err := a11y.RelevantSpeechMonitor(ctx, cr, tconn, ed)
	if err != nil {
		return SetUpData{}, errors.Wrap(err, "failed to connect to the TTS background page")
	}
	cleanUpFuncs = append(cleanUpFuncs, func() error {
		sm.Close()
		return nil
	})

	if err := cvconn.SetVoice(ctx, vd); err != nil {
		return SetUpData{}, errors.Wrap(err, "failed to set the ChromeVox voice")
	}

	if err := a11y.SetTTSRate(ctx, tconn, 1.0); err != nil {
		return SetUpData{}, errors.Wrap(err, "failed to change TTS rate")
	}

	// Wait for ChromeVox to focus the root web area.
	rootWebArea := nodewith.Role(role.RootWebArea).First()
	if err = cvconn.WaitForFocusedNode(ctx, tconn, rootWebArea); err != nil {
		return SetUpData{}, errors.Wrap(err, "failed to wait for initial ChromeVox focus")
	}

	return SetUpData{cvconn, sm, tearDown}, nil
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
func (cv *Conn) SetVoice(ctx context.Context, vd a11y.VoiceData) error {
	voices, err := a11y.Voices(ctx, cv.Conn)
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
