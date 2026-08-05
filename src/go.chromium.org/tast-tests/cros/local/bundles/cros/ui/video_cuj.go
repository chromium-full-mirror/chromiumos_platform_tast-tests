// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast-tests/cros/local/ui/videocuj"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: VideoCUJ,
		Desc: "Measures the performance of a critical user journey of watching a video",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"vincentchiang@chromium.org",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		SoftwareDeps: []string{"chrome"},
		Data:         []string{cujrecorder.SystemTraceConfigFile},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Timeout:      15 * time.Minute,
		Params: []testing.Param{
			{
				ExtraAttr: []string{"group:cuj"},
				Fixture:   "loggedInToCUJUser",
				Val:       videocuj.TestParam{},
			},
			// Experimental variants.
			{
				Name:      "field_trials",
				ExtraAttr: []string{"group:cuj", "cuj_experimental"},
				Fixture:   "loggedInToCUJUserWithFieldTrials",
				Val:       videocuj.TestParam{},
			},
			{
				Name:    "battery_saver",
				Fixture: "loggedInToCUJUserWithBatterySaver",
				Val:     videocuj.TestParam{},
			},
			// TODO(b/292249282): Remove when Vulkan is launched on brya, volteer, and skyrim
			{
				Name:              "vulkan",
				Fixture:           "loggedInToCUJUserVulkan",
				ExtraAttr:         []string{"group:cuj"},
				ExtraHardwareDeps: hwdep.D(hwdep.Model("redrix", "drobit", "frostflow")),
				Val:               videocuj.TestParam{},
			},
			{
				Name: "coral",
				// No attrs because this test can only be triggered manually currently.
				Fixture:           "loggedInToCUJUserWithCoralEnabled",
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				Val:               videocuj.TestParam{},
			},
			{
				Name: "extra_tabs",
				// No attrs because this test is only for comparing with extra_tabs_coral, which can only be triggered manually currently.
				Fixture: "loggedInToCUJUser",
				Val: videocuj.TestParam{
					ExtraURLsToOpen: []string{
						"chrome://device-log",
						"chrome://version",
						"chrome://histograms",
						"chrome://settings",
					},
				},
			},
			{
				Name: "extra_tabs_coral",
				// No attrs because this test can only be triggered manually currently.
				Fixture:           "loggedInToCUJUserWithCoralEnabled",
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				Val: videocuj.TestParam{
					ExtraURLsToOpen: []string{
						"chrome://device-log",
						"chrome://version",
						"chrome://histograms",
						"chrome://settings",
					},
				},
			},
		},
	})
}

func VideoCUJ(ctx context.Context, s *testing.State) {
	cuj.WriteMetadataFile(ctx, s.TestName())
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	testParam := s.Param().(videocuj.TestParam)
	testParam.SystemTraceConfigPath = s.DataPath(cujrecorder.SystemTraceConfigFile)

	if err := videocuj.Run(ctx, cr, s.OutDir(), testParam); err != nil {
		s.Fatal("Failed to run VideoCUJ: ", err)
	}
}
