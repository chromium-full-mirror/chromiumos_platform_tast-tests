// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package phonehub

import (
	"context"
	"strings"
	"time"

	cdcommon "go.chromium.org/tast-tests/cros/common/cros/crossdevice"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/crossdevice"
	"go.chromium.org/tast-tests/cros/local/chrome/crossdevice/phonehub"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RecentTabs,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that recently opened Chrome tabs on Android appear in Phone Hub",
		Contacts: []string{
			"chromeos-cross-device-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"pushi@google.com",
		},
		BugComponent: "b:1131837",
		Attr:         []string{"group:cross-device", "cross-device_phonehub"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{
			{
				Fixture:           "crossdeviceOnboardedAllFeaturesRerun",
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(cdcommon.UnstableModels...)),
				Val:               browser.TypeAsh,
			},
			{
				Name:              "unstable",
				Fixture:           "crossdeviceOnboardedAllFeaturesRerun",
				ExtraHardwareDeps: hwdep.D(hwdep.Model(cdcommon.UnstableModels...)),
				Val:               browser.TypeAsh,
			},
			{
				Name:      "floss",
				Fixture:   "crossdeviceOnboardedAllFeaturesFlossRerun",
				Val:       browser.TypeAsh,
				ExtraAttr: []string{"cross-device_floss"},
			},
			// TODO(b/260599791): According to go/lacros-phonehub-sync, Ash web browser
			// is meant to be disabled when using Lacros. Phonehub on lacros needs a workaround
			// to access data from Sessions Sync Data before the following tests can be
			// re-enabled.
			/*
				{
					Name:              "lacros",
					Fixture:           "lacrosCrossdeviceOnboardedAllFeatures",
					ExtraAttr:         []string{"cross-device_lacros"},
					ExtraSoftwareDeps: []string{"lacros"},
					Val:               browser.TypeLacros,
				},
				{
					Name:              "lacros_floss",
					Fixture:           "lacrosCrossdeviceOnboardedAllFeaturesFloss",
					ExtraAttr:         []string{"cross-device_lacros", "cross-device_floss"},
					ExtraSoftwareDeps: []string{"lacros"},
					Val:               browser.TypeLacros,
				},
			*/
		},
	})
}

// RecentTabs tests that recently opened Chrome tabs on Android appear in Phone Hub and can be opened in Chrome on CrOS.
func RecentTabs(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*crossdevice.FixtData).Chrome
	tconn := s.FixtValue().(*crossdevice.FixtData).TestConn
	androidDevice := s.FixtValue().(*crossdevice.FixtData).AndroidDevice

	// Opt in to Chrome Sync. This is required for recent tabs to appear in Phone Hub.
	if err := androidDevice.EnableChromeSync(ctx); err != nil {
		s.Fatal("Failed to enable Chrome Sync on Android: ", err)
	}

	if err := phonehub.Show(ctx, tconn); err != nil {
		s.Fatal("Failed to open Phone Hub: ", err)
	}
	defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "recent_tabs_failure")

	// Check that no recent tabs are displayed initially.
	ui := uiauto.New(tconn)
	if err := ui.EnsureGoneFor(phonehub.RecentTabChipFinder, 10*time.Second)(ctx); err != nil {
		s.Fatal("Failed to confirm that recent tabs are initially empty: ", err)
	}

	// Open some tabs on Android.
	urls := []string{"chromium.org", "google.com"}
	for _, url := range urls {
		if err := androidDevice.LaunchChromeAtURL(ctx, url); err != nil {
			s.Fatalf("Failed open Chrome tab for URL %v: %v", url, err)
		}
	}

	// Wait for the chips in Phone Hub to update with the recently opened tabs.
	if err := testing.Poll(ctx, func(context.Context) error {
		chips, err := phonehub.RecentTabChips(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to get recent tab chips")
		}
		for _, url := range urls {
			found := false
			for _, chip := range chips {
				if strings.Contains(chip.URL, url) {
					found = true
				}
			}
			if !found {
				return errors.Errorf("chip for URL %v not found", url)
			}
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: time.Second}); err != nil {
		s.Fatal("Timed out waiting for Phone Hub recent tab chips to update: ", err)
	}

	// Click each of the chips and make sure they open Chrome to the correct URL.
	chips, err := phonehub.RecentTabChips(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get recent tab chips: ", err)
	}
	bt := s.Param().(browser.Type)
	for _, chip := range chips {
		if err := phonehub.Show(ctx, tconn); err != nil {
			s.Fatal("Failed to open Phone Hub: ", err)
		}
		if err := ui.LeftClick(chip.Finder)(ctx); err != nil {
			s.Fatalf("Failed to click chip for %v: %v", chip.URL, err)
		}
		br, brCleanUp, err := browserfixt.Connect(ctx, cr, bt)
		if err != nil {
			s.Fatalf("Failed to connect to active browser for %v: %v", chip.URL, err)
		}
		defer brCleanUp(ctx)
		c, err := br.NewConnForTarget(ctx, chrome.MatchTargetURL(chip.URL))
		if err != nil {
			s.Fatalf("Failed to find browser window for %v: %v", chip.URL, err)
		}
		defer c.Close()
		defer c.CloseTarget(ctx)
	}
}
