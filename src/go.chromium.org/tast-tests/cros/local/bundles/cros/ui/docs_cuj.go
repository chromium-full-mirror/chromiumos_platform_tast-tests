// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
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
			// TODO(b/343320265): Remove after cbx device launches the feature.
			{
				Name:              "image_indexing",
				Val:               docscuj.TestParam{},
				Fixture:           "loggedInToCUJUserWithImageICA",
				ExtraData:         []string{launcher.ImageSearchPowerTestPictureName},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
			},
		},
	})
}

func DocsCUJ(ctx context.Context, s *testing.State) {
	cuj.WriteMetadataFile(ctx, s.TestName())

	testParam := s.Param().(docscuj.TestParam)
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
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

	if _, err := docscuj.Run(ctx, cr, testParam, s.OutDir(), traceConfigPath, s.TestName()); err != nil {
		s.Fatal("Failed to run DocsCUJ: ", err)
	}
}
