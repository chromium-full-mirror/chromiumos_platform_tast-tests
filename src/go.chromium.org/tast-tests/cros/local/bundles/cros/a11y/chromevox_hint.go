// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package a11y provides functions to assist with interacting with accessibility
// features and settings.
package a11y

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/a11y/tts"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChromevoxHint,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "A test that verifies the behavior of the ChromeVox hint in OOBE. This is a feature that is activated after 20s of idle on the OOBE welcome page. After 20s of idle, we show a dialog and give a spoken announcement with instructions for activating ChromeVox",
		Contacts: []string{
			"chromeos-a11y-eng@google.com", // Mailing list
			"akihiroota@chromium.org",      // Test author
		},
		BugComponent: "b:1272895",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.Keyboard()),
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		Params: []testing.Param{{
			Name: "accept_dialog",
			Val:  true,
		}, {
			Name: "dismiss_dialog",
			Val:  false,
		}},
	})
}

func ChromevoxHint(ctx context.Context, s *testing.State) {
	// Shorten deadline to leave time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// This feature is disabled in dev mode, so pass in the flag to explicitly enable it.
	cr, err := chrome.New(ctx, chrome.NoLogin(), chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")), chrome.ExtraArgs("--enable-oobe-chromevox-hint-timer-for-dev-mode"))
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Mute the device to avoid noisiness.
	if err := crastestclient.Mute(ctx); err != nil {
		s.Fatal("Failed to mute: ", err)
	}

	defer crastestclient.Unmute(cleanupCtx)

	oobeConn, err := cr.WaitForOOBEConnection(ctx)
	if err != nil {
		s.Fatal("Failed to create OOBE connection: ", err)
	}
	defer oobeConn.Close()

	// Wait for the welcome screen to be shown.
	if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.WelcomeScreen.isVisible()"); err != nil {
		s.Fatal("Failed to wait for the welcome screen to be visible: ", err)
	}

	// Get a speech monitor for the Google TTS engine.
	ed := tts.GoogleTTSEngine()
	sm, err := tts.RelevantSpeechMonitor(ctx, cr, tconn, ed)
	if err != nil {
		s.Fatal("Failed to connect to the TTS background page: ", err)
	}
	defer sm.Close()

	enabled, err := a11y.FeatureEnabled(ctx, tconn, a11y.SpokenFeedback)
	if err != nil {
		s.Fatal("Failed to get the spoken feedback enabled state: ", err)
	}
	if enabled {
		s.Fatal("Expected spoken feedback to be disabled, but it was not")
	}

	const textSnippet = "The screen reader on ChromeOS, ChromeVox"
	chromeVoxTextNode := nodewith.NameStartingWith(textSnippet).Role(role.StaticText).Onscreen()
	closeButon := nodewith.Name("Close").Role(role.Button).Onscreen()
	var speechExpectations []tts.SpeechExpectation
	var actions uiauto.Action
	ui := uiauto.New(tconn)
	acceptDialog := s.Param().(bool)

	if acceptDialog {
		// If the dialog is accepted, we should get two speech utterances. The first
		// asks the user if they want to enable ChromeVox. The second is usually the
		// ChromeVox welcome message, which indicates that ChromeVox is on. However,
		// for stability purposes, match any utterance since ChromeVox sometimes
		// speaks other utterances before the welcome message.
		speechExpectations = []tts.SpeechExpectation{
			tts.NewRegexExpectation(textSnippet + "*"),
			tts.NewRegexExpectation(".*"),
		}
		actions = uiauto.Combine("wait for and interact with the ChromeVox hint dialog",
			ui.WithTimeout(30*time.Second).WaitUntilExists(chromeVoxTextNode),
			func(ctx context.Context) error {
				ew, err := input.Keyboard(ctx)
				if err != nil {
					return errors.Wrap(err, "failed to create EventWriter")
				}
				defer ew.Close(ctx)

				// Press the spacebar to accept the ChromeVox hint dialog.
				if err := ew.Accel(ctx, "Space"); err != nil {
					return errors.Wrap(err, "failed to press Space")
				}

				return nil
			},
			ui.WaitUntilGone(chromeVoxTextNode),
		)
	} else {
		// If the dialog is dismissed, we should only get one speech utterance that
		// asks the user if they want to activate ChromeVox.
		speechExpectations = []tts.SpeechExpectation{
			tts.NewRegexExpectation(textSnippet + "*"),
		}
		actions = uiauto.Combine("wait for and interact with the ChromeVox hint dialog",
			ui.WithTimeout(30*time.Second).WaitUntilExists(chromeVoxTextNode),
			ui.LeftClickUntil(closeButon, ui.Gone(chromeVoxTextNode)),
			func(ctx context.Context) error {
				// Verify that ChromeVox is still disabled.
				enabled, err := a11y.FeatureEnabled(ctx, tconn, a11y.SpokenFeedback)
				if err != nil {
					return errors.Wrap(err, "failed to get the spoken feedback enabled state")
				}
				if enabled {
					return errors.New("expected spoken feedback to be disabled, but it was not")
				}

				return nil
			},
		)
	}

	// Execute actions.
	// Wait for the ChromeVox hint dialog to be shown on-screen.
	// Detect this by waiting for the presence of the dialog's static text,
	// since the dialog itself has no name.
	// This should only take 20s from when idle first begins, but allow 30s to
	// avoid any potential race conditions.
	// Once the dialog text appears, either accept or dismiss the hint and
	// wait for the dialog to disappear. This is detected by waiting for the
	// static text to disappear.
	if err := actions(ctx); err != nil {
		s.Fatal("Failed to show and interact with the ChromeVox hint dialog: ", err)
	}

	if acceptDialog {
		// Verify that ChromeVox is enabled. Use a poll since it may take some
		// time for ChromeVox to enable after the hint is accepted.
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			enabled, err := a11y.FeatureEnabled(ctx, tconn, a11y.SpokenFeedback)
			if err != nil {
				return errors.Wrap(err, "failed to get the spoken feedback enabled state")
			}
			if !enabled {
				return errors.New("expected spoken feedback to be enabled, but it was not")
			}

			return nil
		}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
			s.Fatal("Failed to verify that ChromeVox was enabled after accepting the ChromeVox hint: ", err)
		}
	}

	// Lastly, ensure that the correct speech is given by the TTS engine.
	if err := sm.Consume(ctx, speechExpectations); err != nil {
		s.Fatal("Failed to verify the speech utterances for the ChromeVox hint: ", err)
	}
}
