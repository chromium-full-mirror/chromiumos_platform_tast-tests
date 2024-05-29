// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pre

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast/core/testing"
)

const (
	chromeRTC featureType = featureType(uint32(1) << iota)
	// RTCFeatureVsyncDecoding enables WebRTC VsyncDecoding feature.
	RTCFeatureVsyncDecoding

	// keep this at last
	numChromeRTCFeatures = iota
)

func initChromeRTCFixtures() {
	initChromeRTCBaseFixtures()
	initChromeRTCLacrosFixtures()
}

var chromeRTCFixtureGenerator = fixtureGenerator{
	name: "chromeRTC",
}

func initChromeRTCBaseFixtures() {
	combos := []featureComboType{
		comb(chromeRTC),
		comb(chromeRTC, RTCFeatureVsyncDecoding),
	}
	featureMap := map[featureType]featureInfo{
		chromeRTC: {
			"_",
			[]chrome.Option{
				chrome.ExtraArgs(
					// Do not show message center notifications.
					"--suppress-message-center-popups",
					// Disable ARC++.
					"--arc-availability=none",
					// Disable firmware update to stop chrome from executing fwupd that restarts powerd.
					"--disable-features=FirmwareUpdaterApp",
					// Avoid the need to grant camera/microphone permissions.
					"--auto-accept-camera-and-microphone-capture",
					// Chrome automatically selects a tab page whose title contains "test".
					"--auto-select-tab-capture-source-by-title=test",
					// --disable-sync disables test account info sync, eg. Wi-Fi credentials,
					// so that each test run does not remember info from last test run.
					"--disable-sync",
					// Allow 2 windows side by side.
					"--force-tablet-mode=clamshell",
					// Do not attempt to change audio server settings.
					"--use-fake-cras-audio-client-for-dbus",
				),
				chrome.ExtraArgs(chromeWebRTCEncodedFrameArgs...),
				chrome.EnableFeatures(
					// Prefer using constant frame rate for camera streaming.
					"PreferConstantFrameRate",
					// Make noise cancellation available.
					"CrOSLateBootAudioAPNoiseCancellation",
				),
			},
		},
		RTCFeatureVsyncDecoding: {
			"VsyncDecoding",
			[]chrome.Option{
				chrome.EnableFeatures(
					"VSyncDecoding",
				),
			},
		},
	}
	if len(featureMap) != numChromeRTCFeatures {
		panic("Add feature declaration to featureMap")
	}

	chromeRTCFixtureGenerator.initialize(combos, featureMap)
	testing.AddFixture(&testing.Fixture{
		Name:            chromeRTCFixtureGenerator.name,
		Desc:            "Logged into a user session with rtc performance settings",
		Contacts:        []string{"chromeos-rtc@google.com"},
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return s.Param().([]chrome.Option), nil
		}),
		Params: chromeRTCFixtureGenerator.genParams(),
	})
}

var chromeRTCLacrosFixtureGenerator = fixtureGenerator{
	name: "chromeRTCLacros",
}

func initChromeRTCLacrosFixtures() {
	combos := []featureComboType{
		comb(chromeRTC),
		comb(chromeRTC, RTCFeatureVsyncDecoding),
	}
	featureMap := map[featureType]featureInfo{
		chromeRTC: {
			"_",
			[]chrome.Option{
				chrome.ExtraArgs(
					// Do not show message center notifications.
					"--suppress-message-center-popups",
					// Disable ARC++.
					"--arc-availability=none",
					// Disable firmware update to stop chrome from executing fwupd that restarts powerd.
					"--disable-features=FirmwareUpdaterApp",
					// Avoid the need to grant camera/microphone permissions.
					"--auto-accept-camera-and-microphone-capture",
					// Chrome automatically selects a tab page whose title contains "test".
					"--auto-select-tab-capture-source-by-title=test",
					// --disable-sync disables test account info sync, eg. Wi-Fi credentials,
					// so that each test run does not remember info from last test run.
					"--disable-sync",
					// Allow 2 windows side by side.
					"--force-tablet-mode=clamshell",
					// Do not attempt to change audio server settings.
					"--use-fake-cras-audio-client-for-dbus",
				),
				chrome.ExtraArgs(chromeWebRTCEncodedFrameArgs...),
				chrome.EnableFeatures(
					// Prefer using constant frame rate for camera streaming.
					"PreferConstantFrameRate",
					// Make noise cancellation available.
					"CrOSLateBootAudioAPNoiseCancellation",
				),
				chrome.LacrosExtraArgs(
					// Avoid the need to grant camera/microphone permissions.
					"--auto-accept-camera-and-microphone-capture",
					// Chrome automatically selects a tab page whose title contains "test".
					"--auto-select-tab-capture-source-by-title=test",
					// --disable-sync disables test account info sync, eg. Wi-Fi credentials,
					// so that each test run does not remember info from last test run.
					"--disable-sync",
					// Do not attempt to change audio server settings.
					"--use-fake-cras-audio-client-for-dbus",
				),
				chrome.LacrosExtraArgs(chromeWebRTCEncodedFrameArgs...),
				chrome.LacrosEnableFeatures(
					// Prefer using constant frame rate for camera streaming.
					"PreferConstantFrameRate",
					// Make noise cancellation available.
					"CrOSLateBootAudioAPNoiseCancellation",
				),
			},
		},
		RTCFeatureVsyncDecoding: {
			"VsyncDecoding",
			[]chrome.Option{
				chrome.EnableFeatures(
					"VSyncDecoding",
				),
				chrome.LacrosEnableFeatures(
					"VSyncDecoding",
				),
			},
		},
	}

	chromeRTCLacrosFixtureGenerator.initialize(combos, featureMap)

	testing.AddFixture(&testing.Fixture{
		Name:            chromeRTCLacrosFixtureGenerator.name,
		Desc:            "Logged into a user session with rtc performance settings (lacros)",
		Contacts:        []string{"chromeos-rtc@google.com"},
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(s.Param().([]chrome.Option)...)).Opts()
		}),
		Params: chromeRTCLacrosFixtureGenerator.genParams(),
	})
}

// ChromeRTCFixture returns a string of fixture used in chrome RTC tests.
func ChromeRTCFixture(features ...featureType) string {
	return chromeRTCFixtureGenerator.getFixture(add(comb(features...), chromeRTC))
}

// ChromeRTCLacrosFixture returns a string of fixture used in chrome RTC tests running with lacros.
func ChromeRTCLacrosFixture(features ...featureType) string {
	return chromeRTCLacrosFixtureGenerator.getFixture(add(comb(features...), chromeRTC))
}
