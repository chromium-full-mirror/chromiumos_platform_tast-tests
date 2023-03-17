// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package sts (Select-to-Speak) provides functions to assist with interacting with the Chrome OS Select-to-Speak feature.
package sts

import (
	"context"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/a11y"
	"chromiumos/tast/local/audio/crastestclient"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
)

// SetUpData contains useful objects for Select-to-Speak tests and is
// returned by SetUp. Most notably, TearDown is a function that should
// be run in a defer statement by the caller to properly tear-down
// Select-to-Speak. SM will remain alive until TearDown is called.
type SetUpData struct {
	Ctx      context.Context
	SM       *a11y.SpeechMonitor
	TearDown func() error
}

// SetUp executes common Select to Speak setup code. Returns a SetUpData. When
// the error is nil, SetUpData will contain a non-nil TearDown function. The
// caller should call it in a defer statement for proper cleanup.
func SetUp(ctx context.Context, cr *chrome.Chrome, ed a11y.TTSEngineData, bt browser.Type, html string) (_ SetUpData, e error) {
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
			return errors.Errorf("failed Select to Speak tear down steps: %q", errs)
		}

		return nil
	}

	// Tears down Select to Speak if SetUp encountered an error.
	defer func() {
		if e != nil {
			tearDown()
		}
	}()

	// Shorten deadline to leave time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	cleanUpFuncs = append(cleanUpFuncs, func() error {
		cancel()
		return nil
	})

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
		closeBrowser(cleanupCtx)
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

	if err := a11y.SetFeatureEnabled(ctx, tconn, a11y.SelectToSpeak, true); err != nil {
		return SetUpData{}, errors.Wrap(err, "failed to enable Select to Speak")
	}
	cleanUpFuncs = append(cleanUpFuncs, func() error {
		if err := a11y.ClearFeature(cleanupCtx, tconn, a11y.SelectToSpeak); err != nil {
			return errors.Wrap(err, "failed to disable Select to Speak")
		}

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

	if err := a11y.SetTTSRate(ctx, tconn, 1.0); err != nil {
		return SetUpData{}, errors.Wrap(err, "failed to change TTS rate")
	}

	// Note: browser tests have encountered flakes due to this pref not
	// propagating to STS before speech is requested. If this test flakes,
	// it could be caused by the above reason.
	if err := tconn.Call(ctx, nil, "tast.promisify(chrome.settingsPrivate.setPref)", "settings.a11y.select_to_speak_enhanced_voices_dialog_shown", true); err != nil {
		return SetUpData{}, errors.Wrap(err, "failed to set the enhanced voices dialog shown preference to true")
	}

	return SetUpData{ctx, sm, tearDown}, nil
}

// SetSelectionAndActivate sets selection within a node and invokes
// Select-to-Speak to produce speech output. It also verifies speech output
// using the speech monitor.
func SetSelectionAndActivate(ctx context.Context, cr *chrome.Chrome, finder *nodewith.Finder, selStart, selEnd int, sm *a11y.SpeechMonitor, expectations []a11y.SpeechExpectation) error {
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
	if err := a11y.PressKeysAndConsumeExpectations(ctx, sm, []string{"Search+S"}, expectations); err != nil {
		return errors.Wrap(err, "error when invoking Select-to-Speak")
	}

	return nil
}
