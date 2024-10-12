// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pre

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
)

func initChromeCameraPerfFixtures() {
	testing.AddFixture(&testing.Fixture{
		Name:         "chromeCameraPerf",
		Desc:         "Logged into a user session with camera tests-specific setting and without verbose logging that can affect the performance. This fixture should be used only for performance tests",
		Contacts:     []string{"chromeos-camera-eng@google.com"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.ExtraArgs(chromeBypassPermissionsArgs...),
				chrome.ExtraArgs(chromeSuppressNotificationsArgs...),
				chrome.ExtraArgs(
					"--disable-gpu-driver-bug-workarounds",
				),
			}, nil
		}),
		Parent:          "gpuWatchDog",
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
}
