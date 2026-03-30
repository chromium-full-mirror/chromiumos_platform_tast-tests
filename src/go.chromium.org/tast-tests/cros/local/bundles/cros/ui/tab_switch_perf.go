// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/ui/tabswitchperf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: TabSwitchPerf,
		Desc: "Measures the performance of tab-switching",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"ramsaroop@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Attr:         []string{"group:cuj"},
		SoftwareDeps: []string{"chrome"},
		Data:         []string{cujrecorder.SystemTraceConfigFile, tabswitchperf.WPRArchiveName},
		Timeout:      15*time.Minute + tabswitchperf.RecorderCoolDownTimeout,
		Vars:         []string{"ui.TabSwitchPerf.mute"},
		Fixture:      "tabSwitchPerfWPRAsh",
		Params: []testing.Param{
			{
				Val: tabswitchperf.TestParams{
					IsSplitView: false,
				},
			}, {
				Name: "split_view",
				Val: tabswitchperf.TestParams{
					IsSplitView: true,
				},
			},
		},
	})
}

func TabSwitchPerf(ctx context.Context, s *testing.State) {
	cuj.WriteMetadataFile(ctx, s.TestName())

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	outDir := s.OutDir()
	perfettoConfigPath := s.DataPath(cujrecorder.SystemTraceConfigFile)
	params := s.Param().(tabswitchperf.TestParams)

	mute := false
	if val, ok := s.Var("ui.TabSwitchPerf.mute"); ok {
		boolVal, err := strconv.ParseBool(val)
		if err != nil {
			s.Fatal("Cannot parse argument mute: ", err)
		}
		mute = boolVal
	}

	if err := tabswitchperf.Run(ctx, cr, mute, params.IsSplitView, outDir, perfettoConfigPath); err != nil {
		s.Fatal("Failed to run test: ", err)
	}
}
