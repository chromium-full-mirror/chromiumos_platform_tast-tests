// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package sts (Select-to-Speak) provides functions to assist with interacting with the Chrome OS Select-to-Speak feature.
package sts

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/a11y/tts"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/event"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
)

// Conn represents a connection to the Select-to-Speak background page.
type Conn struct {
	*chrome.Conn
}

// NewConn returns a connection to the Select-to-Speak extension's background page.
// If the extension fails to load, the connection will be closed before returning.
// Otherwise the calling function will close the connection.
// Note: this connection will not be allowed to use the enhanced network voices
// TTS engine, as we explicitly disallow it below.
func NewConn(ctx context.Context, c *chrome.Chrome) (_ *Conn, e error) {
	extConn, err := c.NewConnForTarget(ctx, chrome.MatchTargetURL(a11y.SelectToSpeakExtensionURL))
	if err != nil {
		return nil, err
	}

	defer func() {
		if e != nil {
			extConn.Close()
		}
	}()

	// Poll until Select-to-Speak connection finishes loading.
	if err := extConn.WaitForExpr(ctx, `document.readyState === "complete"`); err != nil {
		return nil, errors.Wrap(err, "timed out waiting for Select-to-Speak connection to be ready")
	}

	// Make sure required modules exist and are accessible.
	if err := extConn.Eval(ctx, `(async () => {
		if (!window.selectToSpeak) {
			window.selectToSpeak = (await import('/select_to_speak/select_to_speak_main.js')).selectToSpeak;
		}
	  })()`, nil); err != nil {
		return nil, errors.Wrap(err, "failed to export modules from Select-to-Speak")
	}

	if err := chrome.AddTastLibrary(ctx, extConn); err != nil {
		return nil, errors.Wrap(err, "failed to introduce tast library")
	}

	// Ensure that the enhanced network voices dialog is never shown.
	if err := extConn.Call(ctx, nil, "tast.promisify(chrome.settingsPrivate.setPref)", "settings.a11y.select_to_speak_enhanced_voices_dialog_shown", true); err != nil {
		return nil, errors.Wrap(err, "failed to set the enhanced voices dialog shown preference to true")
	}

	if err := extConn.Eval(ctx, "selectToSpeak.prefsManager_.enhancedVoicesDialogShown_ = true", nil); err != nil {
		return nil, errors.Wrap(err, "failed to set the enhanced network voices dialog local variable to true")
	}

	// Ensure that the enhanced network voices are not enabled.
	if err := extConn.Eval(ctx, "selectToSpeak.prefsManager_.enhancedNetworkVoicesAllowed_ = false", nil); err != nil {
		return nil, errors.Wrap(err, "failed to set the enhanced network voices allowed local variable to false")
	}

	return &Conn{extConn}, nil
}

// SetUp executes common Select to Speak setup code. Returns a TTSFeatureData -
// see the documentation for TTSFeatureData for information on proper cleanup.
func SetUp(ctx context.Context, cr *chrome.Chrome, ed tts.EngineData, bt browser.Type, html string) (tfd a11y.TTSFeatureData, e error) {
	// Tears down Select to Speak if SetUp encountered an error.
	defer func() {
		if e != nil {
			tfd.TDown.TearDown()
		}
	}()

	inputs := a11y.TTSFeatureInputs{CTX: ctx, CR: cr, ED: ed, BT: bt, URL: a11y.URLFromHTML(html), Feature: a11y.SelectToSpeak}
	ttsData, err := a11y.SetUpTTSFeature(inputs)
	if err != nil {
		return ttsData, errors.Wrap(err, "failed to setup common TTS feature state")
	}

	stsConn, err := NewConn(ttsData.CTX, cr)
	if err != nil {
		return ttsData, errors.Wrap(err, "failed to connect to the Select-to-Speak background page")
	}

	ttsData.TDown.Append(func() error {
		stsConn.Close()
		return nil
	})

	return ttsData, nil
}

// SetSelectionAndActivate sets selection within a node and invokes
// Select-to-Speak to produce speech output. It also verifies speech output
// using the speech monitor.
func SetSelectionAndActivate(ctx context.Context, cr *chrome.Chrome, finder *nodewith.Finder, selStart, selEnd int, sm *tts.SpeechMonitor, expectations []tts.SpeechExpectation) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}

	ui := uiauto.New(tconn)
	if err := uiauto.Combine("Set selection and wait for event to propagate",
		ui.WaitUntilExists(finder),
		ui.WaitForEvent(nodewith.Root(), event.DocumentSelectionChanged, ui.Select(finder, selStart, finder, selEnd)),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to set selection")
	}

	// Invoke Select-to-Speak.
	if err := tts.PressKeysAndConsumeExpectations(ctx, sm, []string{"Search+S"}, expectations); err != nil {
		return errors.Wrap(err, "error when invoking Select-to-Speak")
	}

	return nil
}

// ClickAndDragToActivate uses search + click and drag to start select to speak using the bounds of the given node.
func ClickAndDragToActivate(ctx context.Context, cr *chrome.Chrome, finder *nodewith.Finder, sm *tts.SpeechMonitor, expectations []tts.SpeechExpectation) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}

	ui := uiauto.New(tconn).WithTimeout(10 * time.Second)
	if err := ui.WaitUntilExists(finder)(ctx); err != nil {
		return errors.Wrap(err, "timed out waiting for the node to speak")
	}

	bounds, err := ui.Location(ctx, finder)
	if err != nil {
		return errors.Wrap(err, "failed to get the node to speak's location")
	}

	// Open a connection to the keyboard.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "error with creating EventWriter from keyboard")
	}

	// Invoke Select to Speak with search and click and drag.
	if err := kb.AccelPress(ctx, "Search"); err != nil {
		return errors.Wrap(err, "error when pressing the search key")
	}

	if err := mouse.Drag(tconn, bounds.BottomRight(), bounds.TopLeft(), time.Second)(ctx); err != nil {
		return errors.Wrap(err, "failed to click and drag drag the mouse")
	}

	if err := kb.AccelRelease(ctx, "Search"); err != nil {
		return errors.Wrap(err, "error when releasing the search key")
	}

	if err := sm.Consume(ctx, expectations); err != nil {
		return errors.Wrap(err, "error when consuming expectations after search plus click and drag")
	}

	return nil
}
