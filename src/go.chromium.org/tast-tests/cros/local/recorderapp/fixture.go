// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package recorderapp

import (
	"context"
	"time"

	audiofixture "go.chromium.org/tast-tests/cros/local/audio/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/power"
	powersetup "go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/testing"
)

var recorderAppPowerTestOptions = powersetup.PowerTestOptions{
	Audio:              powersetup.DoNotChangeAudio,
	NightLight:         powersetup.DisableNightLight,
	DarkTheme:          powersetup.EnableLightTheme,
	KeyboardBrightness: powersetup.SetKbBrightnessToZero,
}

// addChromeOpts enables conch flag by default and adds chrome options specified in fixture parameter.
func addChromeOpts(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
	chromeOpts := []chrome.Option{
		chrome.EnableFeatures("Conch"),
	}
	if s.Param() != nil {
		extraOpts := s.Param().([]chrome.Option)
		chromeOpts = append(chromeOpts, extraOpts...)
	}
	return chromeOpts, nil
}

// PowerTimeParams are time parameters used in power recording in Recorder App.
var PowerTimeParams = power.TimeParams{Interval: 5 * time.Second, Total: 5 * time.Minute}

// CUJRecordTime is the time used for recording power and latency in CUJ tests.
var CUJRecordTime = 10 * time.Minute

const aloopTimeout = 20 * time.Second

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            "recorderAppPrepared",
		Desc:            "Set necessary settings for launching the Recorder App",
		Contacts:        []string{"chromeos-recorder-app@google.com", "kamchonlathorn@chromium.org"},
		BugComponent:    "b:1522466", // ChromeOS > Platform > Technologies > Audio > Recorder App
		Impl:            &fixture{},
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         "powerAshWithRecorderApp",
		Desc:         "PowerAsh fixture with Recorder App enabled",
		Contacts:     []string{"chromeos-recorder-app@google.com", "kamchonlathorn@chromium.org"},
		BugComponent: "b:1522466", // ChromeOS > Platform > Technologies > Audio > Recorder App
		Impl: powersetup.NewPowerUIFixture(recorderAppPowerTestOptions, powersetup.PowerFixtureOptions{
			EnableGAIALogin: true,
			ExtraOptsFunc:   addChromeOpts,
		}),
		SetUpTimeout:    chrome.GAIALoginTimeout + powersetup.SetUpTimeout,
		ResetTimeout:    powersetup.ResetTimeout,
		TearDownTimeout: powersetup.TearDownTimeout,
		PreTestTimeout:  powersetup.PreTestTimeout,
		PostTestTimeout: powersetup.PostTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         "powerAshGAIAWithRecorderAppAndAloopLoaded",
		Desc:         "PowerAshGAIA fixture with Recorder App enabled and ALSA loopback device configured",
		Contacts:     []string{"chromeos-recorder-app@google.com", "kamchonlathorn@chromium.org"},
		BugComponent: "b:1522466", // ChromeOS > Platform > Technologies > Audio > Recorder App
		Impl: powersetup.NewPowerUIFixture(recorderAppPowerTestOptions, powersetup.PowerFixtureOptions{
			EnableGAIALogin: true,
			ExtraOptsFunc:   addChromeOpts,
		}),
		Parent:          audiofixture.AloopLoaded{Channels: 2}.Instance(),
		SetUpTimeout:    chrome.GAIALoginTimeout + powersetup.SetUpTimeout + aloopTimeout,
		ResetTimeout:    powersetup.ResetTimeout,
		TearDownTimeout: powersetup.TearDownTimeout + aloopTimeout,
		PreTestTimeout:  powersetup.PreTestTimeout + aloopTimeout,
		PostTestTimeout: powersetup.PostTestTimeout,
		Params: []testing.FixtureParam{
			{
				// No additional flags
				Val: []chrome.Option{},
			},
			{
				Name: "japanese_transcription",
				Val:  []chrome.Option{chrome.EnableFeatures("ConchExpandTranscriptionLanguage")},
			},
		},
	})
}

type fixture struct {
	cr *chrome.Chrome
}

// FixtureData is the struct exposed to tests.
type FixtureData struct {
	Chrome *chrome.Chrome
}

func (f *fixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	chromeOpts, _ := addChromeOpts(ctx, s)
	cr, err := chrome.New(ctx, chromeOpts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	f.cr = cr
	return FixtureData{
		Chrome: cr,
	}
}

func (f *fixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := f.cr.Close(ctx); err != nil {
		s.Error("Failed to tear down Chrome: ", err)
	}
	f.cr = nil
}

func (f *fixture) Reset(ctx context.Context) error {
	return nil
}

func (f *fixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *fixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
}
