// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pre

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
)

func initChromeCaptureFixtures() {
	testing.AddFixture(&testing.Fixture{
		Name:         "chromeScreenCapture",
		Desc:         "Logged into a user session with flag so that Chrome always picks the entire screen for getDisplayMedia(), bypassing the picker UI",
		Contacts:     []string{"chromeos-gfx-video@google.com"},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video.
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				chrome.ExtraArgs(`--auto-select-desktop-capture-source=display`),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         "chromeWindowCapture",
		Desc:         "Logged into a user session with flag so that Chrome always picks the Chromium window for getDisplayMedia(), bypassing the picker UI",
		Contacts:     []string{"chromeos-gfx-video@google.com"},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video.
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				chrome.ExtraArgs(`--auto-select-window-capture-source-by-title=test`),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         "chromeTabCapture",
		Desc:         "Logged into a user session with flag so that Chrome always picks the current tab for getDisplayMedia(), bypassing the picker UI",
		Contacts:     []string{"chromeos-gfx-video@google.com"},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video.
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				// Chrome automatically selects a tab page whose title contains "test".
				chrome.ExtraArgs("--auto-select-tab-capture-source-by-title=test"),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         "chromeZeroCopyScreenCapture",
		Desc:         "Logged into a user session with flag so that Chrome always picks the entire screen for getDisplayMedia(), bypassing the picker UI",
		Contacts:     []string{"chromeos-gfx-video@google.com"},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video.
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				chrome.ExtraArgs(`--auto-select-desktop-capture-source=display`),
				chrome.ExtraArgs("--enable-features=ZeroCopyTabCapture"),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         "chromeZeroCopyWindowCapture",
		Desc:         "Logged into a user session with flag so that Chrome always picks the Chromium window for getDisplayMedia(), bypassing the picker UI",
		Contacts:     []string{"chromeos-gfx-video@google.com"},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video.
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				chrome.ExtraArgs(`--auto-select-window-capture-source-by-title=test`),
				chrome.ExtraArgs("--enable-features=ZeroCopyTabCapture"),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         "chromeZeroCopyTabCapture",
		Desc:         "Logged into a user session with flag so that Chrome always picks the current tab for getDisplayMedia(), bypassing the picker UI",
		Contacts:     []string{"chromeos-gfx-video@google.com"},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video.
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				// Chrome automatically selects a tab page whose title contains "test".
				chrome.ExtraArgs("--auto-select-tab-capture-source-by-title=test"),
				chrome.ExtraArgs("--enable-features=ZeroCopyTabCapture"),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:         "chromeZeroCopyTabCaptureAndSWEncoding",
		Desc:         "Logged into a user session with flag so that Chrome always picks the current tab for getDisplayMedia(), bypassing the picker UI with software encoding",
		Contacts:     []string{"chromeos-gfx-video@google.com"},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video.
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return getChromeVideoOptions(
				// Chrome automatically selects a tab page whose title contains "test".
				chrome.ExtraArgs("--auto-select-tab-capture-source-by-title=test"),
				chrome.ExtraArgs("--enable-features=ZeroCopyTabCapture"),
				chrome.ExtraArgs("--disable-accelerated-video-encode"),
			), nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
}
