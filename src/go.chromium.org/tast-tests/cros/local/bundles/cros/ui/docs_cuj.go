// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast-tests/cros/local/ui/docscuj"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DocsCUJ,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Measures the total performance of the critical user journey for Google Docs",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"ramsaroop@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Attr:         []string{"group:cuj"},
		SoftwareDeps: []string{"chrome"},
		Data:         []string{cujrecorder.SystemTraceConfigFile},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Timeout:      20 * time.Minute,
		Params: []testing.Param{
			{
				Val:     browser.TypeAsh,
				Fixture: "loggedInToCUJUser",
			},

			// Experimental variants.
			{
				Name:      "field_trials",
				ExtraAttr: []string{"cuj_experimental"},
				Val:       browser.TypeAsh,
				Fixture:   "loggedInToCUJUserWithFieldTrials",
			},
			{
				Name:      "chromevox",
				ExtraAttr: []string{"cuj_experimental"},
				Val:       browser.TypeAsh,
				Fixture:   "loggedInToCUJUserWithChromeVox",
			},
			// TODO(b/292249282): Remove when Vulkan is launched on brya, volteer, and skyrim.
			{
				Name:              "vulkan",
				Val:               browser.TypeAsh,
				Fixture:           "loggedInToCUJUserVulkan",
				ExtraHardwareDeps: hwdep.D(hwdep.Model("redrix", "drobit", "frostflow")),
			},
			// TODO(b/343320265): Remove after cbx device launches the feature.
			{
				Name:              "image_indexing",
				Val:               browser.TypeAsh,
				Fixture:           "loggedInToCUJUserWithImageICA",
				ExtraData:         []string{launcher.ImageSearchPowerTestPictureName},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
			},
		},
	})
}

func DocsCUJ(ctx context.Context, s *testing.State) {
	cuj.WriteMetadataFile(ctx, s.TestName())

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	bt := s.Param().(browser.Type)
	traceConfigPath := s.DataPath(cujrecorder.SystemTraceConfigFile)

	if strings.HasSuffix(s.TestName(), "image_indexing") {
		user := cr.NormalizedUser()
		testPicturePath := s.DataPath(launcher.ImageSearchPowerTestPictureName)
		cleanup, err := cuj.PrepareImageSearchFiles(ctx, user, testPicturePath, 500)
		if err != nil {
			s.Fatal("Failed to prepare image search files: ", err)
		}
		defer cleanup()
	}

	if _, err := docscuj.Run(ctx, cr, bt, s.OutDir(), traceConfigPath, s.TestName()); err != nil {
		s.Fatal("Failed to run DocsCUJ: ", err)
	}
}
