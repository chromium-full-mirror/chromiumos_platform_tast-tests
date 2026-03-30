// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"path/filepath"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/ui/tabswitchperf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast-tests/cros/local/wpr"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: TabSwitchPerfRecorder,
		// used to record all web traffic via wpr so that later TabSwitchCUJ could run without really talking to real sites
		Desc: "Run tab-switching CUJ test in chromewpr recording mode",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"xiyuan@chromium.org",
			"yichenz@chromium.org",
		},
		// ChromeOS > Software > Performance > TPS
		BugComponent: "b:1045832",
		SoftwareDeps: []string{"chrome"},
		Timeout:      15*time.Minute + tabswitchperf.RecorderCoolDownTimeout,
		Vars:         []string{"ui.TabSwitchPerfRecorder.mute"},
		Pre:          wpr.RecordMode(filepath.Join("/tmp", tabswitchperf.WPRArchiveName)),
	})
}

func TabSwitchPerfRecorder(ctx context.Context, s *testing.State) {
	cr := s.PreValue().(*chrome.Chrome)

	outDir := s.OutDir()
	perfettoConfigPath := s.DataPath(cujrecorder.SystemTraceConfigFile)

	mute := false
	if val, ok := s.Var("ui.TabSwitchPerfRecorder.mute"); ok {
		boolVal, err := strconv.ParseBool(val)
		if err != nil {
			s.Fatal("Cannot parse argument mute: ", err)
		}
		mute = boolVal
	}

	if err := tabswitchperf.Run(ctx, cr, mute, false /* IsSplitView */, outDir, perfettoConfigPath); err != nil {
		s.Fatal("Failed to run test: ", err)
	}
}
