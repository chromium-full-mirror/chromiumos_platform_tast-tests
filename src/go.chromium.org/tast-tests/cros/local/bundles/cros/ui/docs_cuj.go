// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast-tests/cros/local/ui/docscuj"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: DocsCUJ,
		Desc: "Measures the total performance of the critical user journey for Google Docs",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"vincentchiang@chromium.org",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Attr:         []string{"group:cuj"},
		SoftwareDeps: []string{"chrome"},
		Data:         []string{cujrecorder.SystemTraceConfigFile},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Timeout:      20 * time.Minute,
		Params: []testing.Param{
			{
				Val:     docscuj.TestParam{},
				Fixture: "loggedInToCUJUser",
			},
			// Experimental variants.
			{
				Name:      "field_trials",
				Val:       docscuj.TestParam{},
				ExtraAttr: []string{"cuj_experimental"},
				Fixture:   "loggedInToCUJUserWithFieldTrials",
			},
			{
				Name:      "chromevox",
				Val:       docscuj.TestParam{},
				ExtraAttr: []string{"cuj_experimental"},
				Fixture:   "loggedInToCUJUserWithChromeVox",
			},
			{
				Name: "bounce_keys",
				Val: docscuj.TestParam{
					BounceKeysEnabled: true,
				},
				ExtraAttr: []string{"cuj_experimental"},
				Fixture:   "loggedInToCUJUserWithBounceKeys",
			},
			// TODO(b/292249282): Remove when Vulkan is launched on brya, volteer, and skyrim.
			{
				Name:              "vulkan",
				Val:               docscuj.TestParam{},
				Fixture:           "loggedInToCUJUserVulkan",
				ExtraHardwareDeps: hwdep.D(hwdep.Model("redrix", "drobit", "frostflow")),
			},
		},
	})
}

func DocsCUJ(ctx context.Context, s *testing.State) {
	cuj.WriteMetadataFile(ctx, s.TestName())

	testParam := s.Param().(docscuj.TestParam)
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	traceConfigPath := s.DataPath(cujrecorder.SystemTraceConfigFile)

	if _, err := docscuj.Run(ctx, cr, testParam, s.OutDir(), traceConfigPath, s.TestName(), cujrecorder.RecorderOptions{}); err != nil {
		s.Fatal("Failed to run DocsCUJ: ", err)
	}
}
