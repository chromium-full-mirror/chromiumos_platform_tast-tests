// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package sts (Select-to-Speak) provides functions to assist with interacting with the Chrome OS Select-to-Speak feature.
package sts

import (
	"context"

	"chromiumos/tast/errors"
	"chromiumos/tast/local/a11y"
	"chromiumos/tast/local/a11y/tts"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
)

// SetUp executes common Select to Speak setup code. Returns a TTSFeatureData -
// see the documentation for TTSFeatureData for information on proper cleanup.
func SetUp(ctx context.Context, cr *chrome.Chrome, ed tts.EngineData, bt browser.Type, html string) (tfd a11y.TTSFeatureData, e error) {
	// Tears down Select to Speak if SetUp encountered an error.
	defer func() {
		if e != nil {
			tfd.TDown.TearDown()
		}
	}()

	inputs := a11y.TTSFeatureInputs{CTX: ctx, CR: cr, ED: ed, BT: bt, HTML: html, Feature: a11y.SelectToSpeak}
	ttsData, err := a11y.SetUpTTSFeature(inputs)
	if err != nil {
		return ttsData, errors.Wrap(err, "failed to setup common TTS feature state")
	}

	// Note: browser tests have encountered flakes due to this pref not
	// propagating to STS before speech is requested. If this test flakes,
	// it could be caused by the above reason.
	if err := ttsData.TConn.Call(ctx, nil, "tast.promisify(chrome.settingsPrivate.setPref)", "settings.a11y.select_to_speak_enhanced_voices_dialog_shown", true); err != nil {
		return ttsData, errors.Wrap(err, "failed to set the enhanced voices dialog shown preference to true")
	}

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
	if err := uiauto.Combine("Set selection",
		ui.WaitUntilExists(finder),
		ui.Select(finder, selStart, finder, selEnd),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to set selection")
	}

	// Invoke Select-to-Speak.
	if err := tts.PressKeysAndConsumeExpectations(ctx, sm, []string{"Search+S"}, expectations); err != nil {
		return errors.Wrap(err, "error when invoking Select-to-Speak")
	}

	return nil
}
