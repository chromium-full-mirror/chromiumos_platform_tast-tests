// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pre

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast/core/testing"
)

func initChromeRTCFixtures() {
	testing.AddFixture(&testing.Fixture{
		Name:     "chromeRTCPerf",
		Desc:     "Logged into a user session with rtc performance settings",
		Contacts: []string{"chromeos-rtc@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
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
			}, nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeRTCPerfLacros",
		Desc:     "Logged into a user session with rtc performance settings",
		Contacts: []string{"chromeos-rtc@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(
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
				chrome.EnableFeatures(
					// Prefer using constant frame rate for camera streaming.
					"PreferConstantFrameRate",
					// Make noise cancellation available.
					"CrOSLateBootAudioAPNoiseCancellation",
				),
				chrome.LacrosEnableFeatures(
					// Prefer using constant frame rate for camera streaming.
					"PreferConstantFrameRate",
					// Make noise cancellation available.
					"CrOSLateBootAudioAPNoiseCancellation",
				),
			)).Opts()
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.LoginTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
}
