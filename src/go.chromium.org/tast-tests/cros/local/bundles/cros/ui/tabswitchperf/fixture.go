// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tabswitchperf

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/wpr"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: "tabSwitchPerfWPR",
		Desc: "Base fixture for TabSwitchPerf with WPR",
		Contacts: []string{
			"cros-sw-perf@google.com",
		},
		BugComponent:    "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl:            wpr.NewFixture(WPRArchiveName, wpr.Replay),
		SetUpTimeout:    chrome.LoginTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		Data:            []string{WPRArchiveName},
	})

	testing.AddFixture(&testing.Fixture{
		Name: "tabSwitchPerfWPRAsh",
		Desc: "Composed fixture for TabSwitchPerf with WPR",
		Contacts: []string{
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return s.ParentValue().(wpr.FixtValue).FOpt()(ctx, s)
		}),
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		Parent:          "tabSwitchPerfWPR",
	})
}
