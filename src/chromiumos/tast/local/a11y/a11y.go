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
	"regexp"
	"strings"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/audio/crastestclient"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/input"
	"chromiumos/tast/testing"
)

// List of extension IDs and URLs.
const (
	ChromeVoxExtensionURL = "chrome-extension://mndnfokpggljbaajbnioimlmbfngpief/chromevox/background/background.html"
	ESpeakExtensionID     = "dakbfdmgjiabojdgbiljlhgjbokobjpg"
	GoogleTTSExtensionID  = "gjjabgpgjpampikjhjpfhneeoapjbjaf"
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

// SetTTSRate sets the speaking rate of the tts, relative to the default rate (1.0).
func SetTTSRate(ctx context.Context, tconn *chrome.TestConn, rate float64) error {
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

// TTSEngineData represents data for a TTS background page. ExtensionID specifies
// the ID of the background page. If |UseOnSpeakWithAudioStream| is true, we
// listen to onSpeakWithAudioStream when accumulating utterances. Otherwise, we
// listen to onSpeak.
type TTSEngineData struct {
	ExtID                     string
	UseOnSpeakWithAudioStream bool
}

// GoogleTTSEngine is a convenience method that returns TTSEngineData that
// represents the Google text to speech engine.
func GoogleTTSEngine() TTSEngineData {
	return TTSEngineData{
		ExtID:                     GoogleTTSExtensionID,
		UseOnSpeakWithAudioStream: false,
	}
}

// EspeakEngine is a convenience method that returns TTSEngineData that
// represents the eSpeak text to speech engine.
func EspeakEngine() TTSEngineData {
	return TTSEngineData{
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
func RelevantSpeechMonitor(ctx context.Context, c *chrome.Chrome, tconn *chrome.TestConn, engineData TTSEngineData) (*SpeechMonitor, error) {
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
func newSpeechMonitor(ctx context.Context, conn *chrome.Conn, engineData TTSEngineData) (*SpeechMonitor, error) {
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
func ensureTTSEngineLoaded(ctx context.Context, tconn *chrome.TestConn, engineData TTSEngineData) error {
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
		}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
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
	defer ew.Close()

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
func startAccumulatingUtterances(ctx context.Context, conn *chrome.Conn, engineData TTSEngineData) error {
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

	return conn.Eval(ctx, `
	if (!window.testUtterances) {
    window.testUtterances = [];
    chrome.ttsEngine.onSpeak.addListener((utterance, options) => window.testUtterances.push({utterance: utterance, options: options}));
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
	ED      TTSEngineData
	BT      browser.Type
	HTML    string
	Feature Feature
}

// TTSFeatureData contains data and useful objects for an accessibility feature
// that uses TTS. Tconn and SM live until TDown.TearDown() is called.
type TTSFeatureData struct {
	CTX   context.Context
	TConn *chrome.TestConn
	SM    *SpeechMonitor
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

	sm, err := RelevantSpeechMonitor(ctx, cr, tconn, ed)
	if err != nil {
		return newNilTTSFeatureData(tdown), errors.Wrap(err, "failed to connect to the TTS background page")
	}
	tdown.Append(func() error {
		return sm.Close()
	})

	if err := SetTTSRate(ctx, tconn, 1.0); err != nil {
		return newNilTTSFeatureData(tdown), errors.Wrap(err, "failed to change TTS rate")
	}

	return TTSFeatureData{ctx, tconn, sm, tdown}, nil
}
