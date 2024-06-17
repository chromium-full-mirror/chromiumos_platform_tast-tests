// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/ui/tabswitchperf"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TabSwitchPerf,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Measures the performance of tab-switching",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"ramsaroop@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Attr:         []string{"group:cuj"},
		SoftwareDeps: []string{"chrome"},
		Data:         []string{cujrecorder.SystemTraceConfigFile},
		Timeout:      15*time.Minute + cujrecorder.CooldownTimeout,
		Vars: []string{
			"mute",
		},
		Params: []testing.Param{
			{
				ExtraData: []string{tabswitchperf.WPRArchiveName},
				Val: tabswitchperf.TabSwitchParam{
					BrowserType: browser.TypeAsh,
				},
				Fixture: "tabSwitchPerfWPRAsh",
			},
		},
	})
}

func TabSwitchPerf(ctx context.Context, s *testing.State) {
	tabswitchperf.Run(ctx, s)
}
