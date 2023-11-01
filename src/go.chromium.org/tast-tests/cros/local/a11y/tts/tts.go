// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package tts provides functions to assist with interacting with text-to-speech
// engines.
package tts

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// List of extension IDs and URLs.
const (
	ESpeakExtensionID    = "dakbfdmgjiabojdgbiljlhgjbokobjpg"
	GoogleTTSExtensionID = "gjjabgpgjpampikjhjpfhneeoapjbjaf"
)

// SetRate sets the speaking rate of the tts, relative to the default rate (1.0).
func SetRate(ctx context.Context, tconn *chrome.TestConn, rate float64) error {
	return tconn.Call(ctx, nil, "tast.promisify(chrome.settingsPrivate.setPref)", "settings.tts.speech_rate", rate)
}

// VoiceData stores information necessary to identify TTS voices.
type VoiceData struct {
	ExtID  string `json:"extensionId"`
	Locale string `json:"lang"`
	Name   string `json:"voiceName"`
}

// GoogleTTSEnUsVoice is a convenience method that returns VoiceData that
// represents the en-US voice for the Google text to speech engine.
func GoogleTTSEnUsVoice() VoiceData {
	return VoiceData{
		ExtID:  GoogleTTSExtensionID,
		Locale: "en-US",
	}
}

// EspeakElVoice is a convenience method that returns VoiceData that represents
// the el (Greek) voice for the eSpeak text to speech engine. Note: eSpeak does
// not come with an English voice built-in. We use Greek in many tests because
// it's built-in and capable of speaking English words.
func EspeakElVoice() VoiceData {
	return VoiceData{
		ExtID:  ESpeakExtensionID,
		Locale: "el",
	}
}

// Voices returns the current TTS voices which are available.
func Voices(ctx context.Context, conn interface {
	Eval(ctx context.Context, expr string, out interface{}) error
}) ([]VoiceData, error) {
	return voicesImpl(ctx, conn)
}

// voicesImpl implements the Voices for both TestConn and the Conn for Chrome Vox.
// Specifically, the extension connected from conn must have tast library and
// permission to use TTS.
func voicesImpl(
	ctx context.Context,
	conn interface {
		Eval(ctx context.Context, expr string, out interface{}) error
	}) ([]VoiceData, error) {
	var voices []VoiceData
	if err := conn.Eval(ctx, "tast.promisify(chrome.tts.getVoices)()", &voices); err != nil {
		return nil, err
	}
	return voices, nil
}

// EngineData represents data for a TTS background page. ExtensionID specifies
// the ID of the background page. If |UseOnSpeakWithAudioStream| is true, we
// listen to onSpeakWithAudioStream when accumulating utterances. Otherwise, we
// listen to onSpeak.
type EngineData struct {
	ExtID                     string
	UseOnSpeakWithAudioStream bool
}

// GoogleTTSEngine is a convenience method that returns EngineData that
// represents the Google text to speech engine.
func GoogleTTSEngine() EngineData {
	return EngineData{
		ExtID:                     GoogleTTSExtensionID,
		UseOnSpeakWithAudioStream: false,
	}
}

// EspeakEngine is a convenience method that returns EngineData that
// represents the eSpeak text to speech engine.
func EspeakEngine() EngineData {
	return EngineData{
		ExtID:                     ESpeakExtensionID,
		UseOnSpeakWithAudioStream: true,
	}
}

// SpeechMonitor represents a connection to a TTS extension background
// page and is used to verify spoken utterances.
type SpeechMonitor struct {
	conn *chrome.Conn
}

// RelevantSpeechMonitor searches through all possible connections to TTS
// background pages and returns a SpeechMonitor for the one that matches engineData.
// If no TTS background pages can be found, then all
// test connections will be closed before returning. Otherwise the calling
// function will close the connection.
func RelevantSpeechMonitor(ctx context.Context, c *chrome.Chrome, tconn *chrome.TestConn, engineData EngineData) (*SpeechMonitor, error) {
	if err := ensureTTSEngineLoaded(ctx, tconn, engineData); err != nil {
		return nil, errors.Wrap(err, "failed to wait for the TTS engine to load")
	}

	extID := engineData.ExtID
	bgURL := chrome.ExtensionBackgroundPageURL(extID)
	targets, err := c.FindTargets(ctx, chrome.MatchTargetURL(bgURL))
	if err != nil {
		return nil, err
	}

	// Find and connect to the correct TTS extension background page. For each
	// potential background page:
	// 1. Connect to the background page and wait for it to load.
	// 2. Create a SpeechMonitor for the background page.
	// 3. Send a test utterance using chrome.tts.speak().
	// 4. Try to use the SpeechMonitor to consume the utterance. If it doesn't
	// consume, then we can assume that the current target is not the correct one.
	for _, t := range targets {
		var extConn *chrome.Conn
		// A helper function that will return an error if there was an error with
		// creating a SpeechMonitor. If this function returns an error, then we will
		// close extConn and continue to the next target.
		candidateMonitor, err := func() (*SpeechMonitor, error) {
			// Use a poll to connect to the target's background page.
			if err := testing.Poll(ctx, func(ctx context.Context) error {
				var err error
				extConn, err = c.NewConnForTarget(ctx, chrome.MatchTargetID(t.TargetID))
				return err
			}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
				return nil, errors.Wrap(err, "failed to create a connection to candidate TTS background page")
			}

			if err := extConn.WaitForExpr(ctx, `document.readyState === "complete"`); err != nil {
				return nil, errors.Wrap(err, "timed out waiting for the TTS engine background page to load")
			}

			// Create SpeechMonitor.
			candidateMonitor, err := newSpeechMonitor(ctx, extConn, engineData)
			if err != nil {
				return nil, errors.Wrap(err, "could not create a speech monitor")
			}

			return candidateMonitor, nil
		}()

		if err != nil {
			testing.ContextLog(ctx, "skipping background page with ID ", t.TargetID, " for reason: ", err)
			if extConn != nil {
				extConn.Close()
			}
			continue
		}

		// A helper function that will return an error if there was a problem with
		// candidateMonitor. If this function returns an error, then we will close
		// candidateMonitor and continue to the next target.
		if err := func() error {
			// Send a test utterance from tts.
			expr := fmt.Sprintf("chrome.tts.speak('Testing', {extensionId: '%s'});", engineData.ExtID)
			if err := tconn.Eval(ctx, expr, nil); err != nil {
				return errors.Wrap(err, "failed to send a test utterance")
			}

			// Attempt to consume the test utterance.
			if err := candidateMonitor.Consume(ctx, []SpeechExpectation{StringExpectation{"Testing"}}); err != nil {
				return errors.Wrap(err, "failed to consume the test utterance")
			}

			return nil
		}(); err != nil {
			testing.ContextLog(ctx, "skipping background page with ID ", t.TargetID, " for reason: ", err)
			candidateMonitor.Close()
			continue
		}

		// If we get here, then we found the target we are looking for. Return the
		// SpeechMonitor for the target.
		return candidateMonitor, nil
	}

	return nil, errors.New("failed to connect to a TTS background page and create a speech monitor")
}

// newSpeechMonitor connects to a TTS extension, specified by conn and engineData, and
// starts accumulating utterances. Call Consume to compare expected and actual
// utterances.
func newSpeechMonitor(ctx context.Context, conn *chrome.Conn, engineData EngineData) (*SpeechMonitor, error) {
	if err := startAccumulatingUtterances(ctx, conn, engineData); err != nil {
		return nil, errors.Wrap(err, "failed to inject JavaScript to accumulate utterances")
	}
	return &SpeechMonitor{conn}, nil
}

// Eval evaluates JavaScript in the context of the background page represented
// by sm.conn.
func (sm *SpeechMonitor) Eval(ctx context.Context, expr string, out interface{}) error {
	return sm.conn.Eval(ctx, expr, out)
}

// Close closes the connection to the TTS extension's background page.
func (sm *SpeechMonitor) Close() error {
	return sm.conn.Close()
}

// ensureTTSEngineLoaded is a helper function for RelevantSpeechMonitor. It
// ensures that the desired TTS engine is awake and loaded
// before trying to connect to it. Otherwise, we will get errors when trying to
// create a new SpeechMonitor for an engine that hasn't loaded.
func ensureTTSEngineLoaded(ctx context.Context, tconn *chrome.TestConn, engineData EngineData) error {
	// Call chrome.tts.getVoices() and check for a loaded voice with the specified
	// engine ID.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var voices []VoiceData
		if err := tconn.Eval(ctx, "tast.promisify(chrome.tts.getVoices)()", &voices); err != nil {
			return err
		}

		for _, voice := range voices {
			if voice.ExtID == engineData.ExtID {
				return nil
			}
		}

		return errors.New("TTS engine hasn't loaded yet")
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to wait for the TTS engine to load")
	}

	expr := fmt.Sprintf(`new Promise((resolve, reject) => {
		chrome.tts.speak('wakeUp', {
			extensionId: %q,
			onEvent: function(event) {
				if (event.type === chrome.tts.EventType.START ||
						event.type === chrome.tts.EventType.INTERRUPTED) {
					// START suggests that TTS extension is fully ready.
					// If speech was interrupted, then the engine is already awake
					// and ready.
					// It's okay to return at this point.
					resolve();
				} else if (event.type === chrome.tts.EventType.ERROR) {
					reject(new Error(event.errorMessage));
				} else if (event.type === chrome.tts.EventType.CANCELLED) {
					reject(new Error("Unexpected event type: " + event.type));
				}
				// not interested in other event types.
			}},
			function()  {
				if (chrome.runtime.lastError) {
					reject(new Error(chrome.runtime.lastError.message));
				}
			});
	})`, engineData.ExtID)
	if err := tconn.Eval(ctx, expr, nil); err != nil {
		return errors.Wrap(err, "failed to wake up TTS engine")
	}

	return nil
}

// SpeakOptions represents a chrome.tts.SpeakOptions. See:
// https://developer.chrome.com/docs/extensions/reference/ttsEngine/#type-SpeakOptions
type SpeakOptions struct {
	Lang  string  `json:"lang"`
	Pitch float32 `json:"pitch"`
	Rate  float32 `json:"rate"`
}

// UtteranceData defines the data included in a Text to Speech utterance.
type UtteranceData struct {
	Utterance string       `json:"utterance"`
	Options   SpeakOptions `json:"options"`
}

func (ud UtteranceData) String() string {
	return fmt.Sprintf("'%s' (lang: %s, rate: %.2f, pitch: %.2f)", ud.Utterance, ud.Options.Lang, ud.Options.Rate, ud.Options.Pitch)
}

// SpeechExpectation defines an interface for a speech expectation.
type SpeechExpectation interface {
	matches(utteranceData UtteranceData) error
}

// RegexExpectation represents data for a speech expectation, where |expectation|
// is a regular expression.
type RegexExpectation struct {
	expectation string
}

// StringExpectation represents data for a speech expectation, where |expectation|
// is a string.
type StringExpectation struct {
	expectation string
}

// OptionsExpectation represents data for a speech expectation, where |expectation|
// is a string and expectedOptions is a SpeakOptions object with pitch, rate and lang.
type OptionsExpectation struct {
	expectation     string
	expectedOptions SpeakOptions
}

func (re RegexExpectation) matches(utteranceData UtteranceData) error {
	matched, err := regexp.MatchString(re.expectation, utteranceData.Utterance)
	if !matched || err != nil {
		return errors.Errorf("expected pattern: %s does not match utterance: %s", re.expectation, utteranceData.Utterance)
	}

	return nil
}

func (se StringExpectation) matches(utteranceData UtteranceData) error {
	if se.expectation != utteranceData.Utterance {
		return errors.Errorf("expected utterance: %s does not match utterance: %s", se.expectation, utteranceData.Utterance)
	}

	return nil
}

func (oe OptionsExpectation) matches(utteranceData UtteranceData) error {
	var errs []string
	if oe.expectation != utteranceData.Utterance {
		errs = append(errs, fmt.Sprintf("expected utterance: %s does not match utterance: %s", oe.expectation, utteranceData.Utterance))
	}

	if oe.expectedOptions.Lang != utteranceData.Options.Lang {
		errs = append(errs, fmt.Sprintf("expected lang: %s does not match lang: %s", oe.expectedOptions.Lang, utteranceData.Options.Lang))
	}

	if oe.expectedOptions.Pitch != utteranceData.Options.Pitch {
		errs = append(errs, fmt.Sprintf("expected pitch: %.2f does not match pitch: %.2f", oe.expectedOptions.Pitch, utteranceData.Options.Pitch))
	}

	if oe.expectedOptions.Rate != utteranceData.Options.Rate {
		errs = append(errs, fmt.Sprintf("expected rate: %.2f does not match rate: %.2f", oe.expectedOptions.Rate, utteranceData.Options.Rate))
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}

	return nil
}

func (oe OptionsExpectation) String() string {
	return fmt.Sprintf("'%s' (lang: %s, rate: %.2f, pitch: %.2f)", oe.expectation, oe.expectedOptions.Lang, oe.expectedOptions.Pitch, oe.expectedOptions.Rate)
}

// NewRegexExpectation is a convenience method for creating a RegexExpectation
// object.
func NewRegexExpectation(expectation string) RegexExpectation {
	return RegexExpectation{expectation}
}

// NewStringExpectation is a convenience method for creating a StringExpectation
// object.
func NewStringExpectation(expectation string) StringExpectation {
	return StringExpectation{expectation}
}

// NewOptionsExpectation is a convenience method for creating an OptionsExpectation
// object.
func NewOptionsExpectation(utterance, lang string, pitch, rate float32) OptionsExpectation {
	return OptionsExpectation{utterance, SpeakOptions{lang, pitch, rate}}
}

// Consume ensures that the expectations were spoken by the TTS engine. It also
// consumes all utterances accumulated in TTS extension's background page.
// For each expectation we:
// 1. Shift the next spoken utterance off of testUtterances. This ensures that
// we never compare against stale utterances; a spoken utterance is either
// matched or discarded.
// 2. Check if the utterance matches the expectation.
// TODO(b:291094454): Add another method that takes the timeout as a parameter.
func (sm *SpeechMonitor) Consume(ctx context.Context, expectations []SpeechExpectation) error {
	var actual []UtteranceData
	for _, exp := range expectations {
		// Use a poll to allow time for each utterance to be spoken.
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			var utteranceData UtteranceData
			if err := sm.conn.Eval(ctx, "testUtterances.shift()", &utteranceData); err != nil {
				return errors.Wrap(err, "couldn't assign utterance to value of testUtterances.shift() (testUtterances is likely empty)")
			}

			actual = append(actual, utteranceData)
			if err := exp.matches(utteranceData); err != nil {
				return errors.Wrap(err, "expected utterance/pattern hasn't been matched yet")
			}

			return nil
		}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
			return errors.Errorf("expected: %q, but got: %q", expectations, actual)
		}
	}

	return nil
}

// PressKeysAndConsumeExpectations presses keys and ensures that the expectations
// were spoken by the TTS engine.
func PressKeysAndConsumeExpectations(ctx context.Context, sm *SpeechMonitor, keySequence []string, expectations []SpeechExpectation) error {
	// Open a connection to the keyboard.
	ew, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "error with creating EventWriter from keyboard")
	}
	defer ew.Close(ctx)

	for _, keys := range keySequence {
		if err := ew.Accel(ctx, keys); err != nil {
			return errors.Wrapf(err, "error when pressing the keys: %s", keys)
		}
	}

	if err := sm.Consume(ctx, expectations); err != nil {
		return errors.Wrapf(err, "error when consuming expectations after pressing keys: %q", keySequence)
	}

	return nil
}

// startAccumulatingUtterances injects JavaScript into conn's background
// page to accumulate spoken utterances. This function only supports the Google
// TTS and eSpeak TTS engines. If engineData.UseOnSpeakWithAudioStream is true,
// then we will listen to onSpeakWithAudioStream when accumulating utterances.
// Otherwise, we will listen to onSpeak.
func startAccumulatingUtterances(ctx context.Context, conn *chrome.Conn, engineData EngineData) error {
	extID := engineData.ExtID
	if extID != GoogleTTSExtensionID && extID != ESpeakExtensionID {
		return errors.Errorf("could not inject JavaScript into the background page of extension with id, %s, since it doesn't match the Google TTS or eSpeak extension ID", extID)
	}
	if engineData.UseOnSpeakWithAudioStream {
		return conn.Eval(ctx, `
			if (!window.testUtterances) {
		    window.testUtterances = [];
		    chrome.ttsEngine.onSpeakWithAudioStream.addListener((utterance, options) => window.testUtterances.push({utterance: utterance, options: options}));
		  }
	`, nil)
	}

	// Google TTS has a built-in member that can be modified to signal that this
	// is a Tast test. Once window.testUtterances is defined and isTastTest_ is
	// set to true, Google TTS will begin populating window.testUtterances with
	// utterance data.
	return conn.Eval(ctx, `
	if (!window.testUtterances) {
    window.testUtterances = [];
  }

  if (!window.engine.isTastTest_) {
    window.engine.isTastTest_ = true;
  }
`, nil)
}

// SendSpeechRequest is a function that can be used to directly invoke speech
// from a TTS engine. This is useful for testing and verifying speech output
// without having to setup an accessibility feature e.g. ChromeVox or
// Select-to-Speak.
func (sm *SpeechMonitor) SendSpeechRequest(ctx context.Context, text string) error {
	return sm.Eval(ctx, fmt.Sprintf("chrome.tts.speak('%s')", text), nil)
}
