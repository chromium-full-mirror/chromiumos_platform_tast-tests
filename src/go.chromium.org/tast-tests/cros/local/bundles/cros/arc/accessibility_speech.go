// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/a11y/chromevox"
	"go.chromium.org/tast-tests/cros/local/a11y/tts"
	"go.chromium.org/tast-tests/cros/local/arc"
	arca11y "go.chromium.org/tast-tests/cros/local/bundles/cros/arc/a11y"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type expectedSpeechLog struct {
	CheckBox                     []tts.SpeechExpectation
	CheckBoxWithStateDescription []tts.SpeechExpectation
	SeekBar                      []tts.SpeechExpectation
	Slider                       []tts.SpeechExpectation
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         AccessibilitySpeech,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks ChromeVox reads Android elements as expected",
		Contacts:     []string{"arc-framework+tast@google.com", "hirokisato@chromium.org", "dtseng@chromium.org"},
		// ChromeOS > Software > ARC++ > Framework > Accessibility
		BugComponent: "b:165222",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "arcBootedWithoutUIAutomator",
		Timeout:      4 * time.Minute,
		Params: []testing.Param{{
			Val: expectedSpeechLog{
				CheckBox: []tts.SpeechExpectation{
					tts.NewStringExpectation("CheckBox"),
					tts.NewStringExpectation("Check box"),
					tts.NewStringExpectation("Not checked"),
					tts.NewStringExpectation("Press Search plus Space to toggle"),
				},
				CheckBoxWithStateDescription: []tts.SpeechExpectation{
					tts.NewStringExpectation("CheckBoxWithStateDescription"),
					tts.NewStringExpectation("Check box"),
					tts.NewStringExpectation("Not checked"),
					tts.NewStringExpectation("Press Search plus Space to toggle"),
				},
				SeekBar: []tts.SpeechExpectation{
					tts.NewStringExpectation("seekBar"),
					tts.NewStringExpectation("Slider"),
					tts.NewStringExpectation("25"),
					tts.NewStringExpectation("Min 0"),
					tts.NewStringExpectation("Max 100"),
				},
				Slider: []tts.SpeechExpectation{
					tts.NewStringExpectation("Slider"),
					tts.NewStringExpectation("3"),
					tts.NewStringExpectation("Min 0"),
					tts.NewStringExpectation("Max 10"),
				},
			},
			ExtraSoftwareDeps: []string{"android_p"},
		}, {
			Name: "vm",
			Val: expectedSpeechLog{
				CheckBox: []tts.SpeechExpectation{
					tts.NewStringExpectation("CheckBox"),
					tts.NewStringExpectation("Check box"),
					tts.NewStringExpectation("not checked"),
					tts.NewStringExpectation("Press Search plus Space to toggle"),
				},
				CheckBoxWithStateDescription: []tts.SpeechExpectation{
					tts.NewStringExpectation("CheckBoxWithStateDescription"),
					tts.NewStringExpectation("Check box"),
					tts.NewStringExpectation("state description not checked"),
					tts.NewStringExpectation("Press Search plus Space to toggle"),
				},
				SeekBar: []tts.SpeechExpectation{
					tts.NewStringExpectation("seekBar"),
					tts.NewStringExpectation("Slider"),
					tts.NewStringExpectation("state description 25"),
					tts.NewStringExpectation("Min 0"),
					tts.NewStringExpectation("Max 100"),
				},
				Slider: []tts.SpeechExpectation{
					tts.NewStringExpectation("Slider"),
					tts.NewStringExpectation("30 percent"),
					tts.NewStringExpectation("Min 0"),
					tts.NewStringExpectation("Max 10"),
				},
			},
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
	})
}

type axSpeechTestStep struct {
	keys         string
	expectations []tts.SpeechExpectation
}

func AccessibilitySpeech(ctx context.Context, s *testing.State) {
	// TODO(b:146844194): Add test for EditTextActivity.
	MainActivityTestSteps := []axSpeechTestStep{
		{
			"Search+Right",
			[]tts.SpeechExpectation{tts.NewStringExpectation("Main Activity")},
		}, {
			"Search+Right",
			[]tts.SpeechExpectation{
				tts.NewStringExpectation("OFF"),
				tts.NewStringExpectation("Toggle Button"),
				tts.NewStringExpectation("Not pressed"),
				tts.NewStringExpectation("Press Search plus Space to toggle"),
			},
		}, {
			"Search+Right",
			s.Param().(expectedSpeechLog).CheckBox,
		}, {
			"Search+Right",
			s.Param().(expectedSpeechLog).CheckBoxWithStateDescription,
		}, {
			"Search+Right",
			s.Param().(expectedSpeechLog).SeekBar,
		}, {
			"Search+Right",
			s.Param().(expectedSpeechLog).Slider,
		}, {
			"Search+Right",
			[]tts.SpeechExpectation{
				tts.NewRegexExpectation("(?i)ANNOUNCE"),
				tts.NewStringExpectation("Button"),
				tts.NewStringExpectation("Press Search plus Space to activate"),
			},
		}, {
			"Search+Space",
			[]tts.SpeechExpectation{tts.NewStringExpectation("test announcement")},
		}, {
			"Search+Right",
			[]tts.SpeechExpectation{
				tts.NewRegexExpectation("(?i)CLICK TO SHOW TOAST"),
				tts.NewStringExpectation("Button"),
				tts.NewStringExpectation("Press Search plus Space to activate"),
			},
		}, {
			"Search+Space",
			[]tts.SpeechExpectation{tts.NewStringExpectation("test toast")},
		},
	}

	LiveRegionActivityTestSteps := []axSpeechTestStep{
		{
			"Search+Right",
			[]tts.SpeechExpectation{tts.NewStringExpectation("Live Region Activity")},
		}, {
			"Search+Right",
			[]tts.SpeechExpectation{
				tts.NewRegexExpectation("(?i)CHANGE POLITE LIVE REGION"),
				tts.NewStringExpectation("Button"),
				tts.NewStringExpectation("Press Search plus Space to activate"),
			},
		}, {
			"Search+Space",
			[]tts.SpeechExpectation{
				tts.NewStringExpectation("Updated polite text"),
			},
		}, {
			"Search+Right",
			[]tts.SpeechExpectation{
				tts.NewRegexExpectation("(?i)CHANGE ASSERTIVE LIVE REGION"),
				tts.NewStringExpectation("Button"),
				tts.NewStringExpectation("Press Search plus Space to activate"),
			},
		}, {
			"Search+Space",
			[]tts.SpeechExpectation{
				tts.NewStringExpectation("Updated assertive text"),
			},
		},
	}

	ActionActivityTestSteps := []axSpeechTestStep{
		{
			"Search+Right",
			[]tts.SpeechExpectation{tts.NewStringExpectation("Action Activity")},
		}, {
			"Search+Right",
			[]tts.SpeechExpectation{
				tts.NewRegexExpectation("(?i)LONG CLICK"),
				tts.NewStringExpectation("Button"),
				tts.NewStringExpectation("Press Search plus Shift plus Space to long click"),
			},
		}, {
			"Search+Shift+Space",
			[]tts.SpeechExpectation{tts.NewStringExpectation("long clicked")},
		}, {
			"Search+Right",
			[]tts.SpeechExpectation{
				tts.NewRegexExpectation("(?i)LABEL"), tts.NewStringExpectation("Button"),
				tts.NewStringExpectation("Press Search plus Space to perform click"),
				tts.NewStringExpectation("Press Search plus Shift plus Space to perform long click"),
			},
		}, {
			"Search+Right",
			[]tts.SpeechExpectation{
				tts.NewRegexExpectation("(?i)CUSTOM ACTION"), tts.NewStringExpectation("Button"),
				tts.NewStringExpectation("Actions available. Press Search plus Ctrl plus A to view"),
			},
		},
	}

	testActivities := []arca11y.TestActivity{arca11y.MainActivity, arca11y.LiveRegionActivity, arca11y.ActionActivity}

	speechTestSteps := map[arca11y.TestActivity][]axSpeechTestStep{
		arca11y.MainActivity:       MainActivityTestSteps,
		arca11y.LiveRegionActivity: LiveRegionActivityTestSteps,
		arca11y.ActionActivity:     ActionActivityTestSteps,
	}

	testFunc := func(ctx context.Context, cvconn *chromevox.Conn, tconn *chrome.TestConn, currentActivity arca11y.TestActivity) error {
		if err := tts.SetRate(ctx, tconn, 5.0); err != nil {
			s.Fatal("Failed to change TTS rate: ", err)
		}
		defer tts.SetRate(ctx, tconn, 1.0)

		if err := cvconn.SetVoice(ctx, tts.VoiceData{
			ExtID:  tts.GoogleTTSExtensionID,
			Locale: "en-US",
		}); err != nil {
			return errors.Wrap(err, "failed to set the ChromeVox voice")
		}

		sm, err := tts.RelevantSpeechMonitor(ctx, s.FixtValue().(*arc.PreData).Chrome, tconn, tts.EngineData{ExtID: tts.GoogleTTSExtensionID, UseOnSpeakWithAudioStream: false})
		if err != nil {
			return errors.Wrap(err, "failed to connect to the TTS background page")
		}
		defer sm.Close()

		testSteps := speechTestSteps[currentActivity]
		for _, testStep := range testSteps {
			if err := tts.PressKeysAndConsumeExpectations(ctx, sm, []string{testStep.keys}, testStep.expectations); err != nil {
				return errors.Wrapf(err, "failure on the step %+v", testStep)
			}
		}
		return nil
	}
	arca11y.RunTest(ctx, s, testActivities, testFunc)
}
