// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package personalization

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/ambient"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/personalization"

	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: NewUserDefaultWallpaper,
		Desc: "Test which wallpaper appears after OOBE",
		Contacts: []string{
			"cros-p13n-eng@google.com",
			"chromeos-consumer-engprod@google.com",
		},
		// ChromeOS > Software > Personalization
		BugComponent: "b:1006527",
		SoftwareDeps: []string{"chrome"},
		Timeout:      5 * time.Minute,
		Params: []testing.Param{
			{
				ExtraAttr:         []string{"group:cbx", "cbx_feature_enabled"},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1), hwdep.SkipOnModel(ambient.JupiterScreenSaverDefaultModel)),
				ExtraTestBedDeps:  []string{tbdep.Cbx(true)},
				Fixture:           "chromeLoggedIn",
				Name:              "cbx",
				Val:               "Dawn to dark - Cloud Flow",
			},
			{
				ExtraAttr:         []string{"group:cbx", "cbx_feature_enabled"},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1), hwdep.Model(ambient.JupiterScreenSaverDefaultModel)),
				ExtraTestBedDeps:  []string{tbdep.Cbx(true)},
				Fixture:           "chromeLoggedIn",
				Name:              "cbx_jupiter",
				Val:               "Dawn to dark - Jupiter",
			},
			{
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(0)),
				ExtraTestBedDeps:  []string{tbdep.Cbx(false)},
				Fixture:           "chromeLoggedIn",
				Name:              "non_cbx",
				Val:               "Default Wallpaper",
			},
			{
				ExtraAttr:         []string{"group:cbx", "cbx_feature_enabled"},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1), hwdep.SkipOnModel(ambient.JupiterScreenSaverDefaultModel)),
				ExtraTestBedDeps:  []string{tbdep.Cbx(true)},
				Fixture:           fixture.PostDemoModeOOBESkipBothComponentsProd,
				Name:              "demo_cbx",
				Val:               "Dawn to dark - Cloud Flow",
			},
		},
	})
}

func NewUserDefaultWallpaper(ctx context.Context, s *testing.State) {
	var cr *chrome.Chrome
	if hasChrome, ok := s.FixtValue().(chrome.HasChrome); ok {
		cr = hasChrome.Chrome()
	} else {
		// demo mode fixture does not create a Chrome instance.
		var err error
		cr, err = chrome.New(ctx,
			chrome.NoLogin(),
			chrome.KeepEnrollment(),
			chrome.DMSPolicy(policy.DMServerProdURL),
			// TODO: b/413709868 - enable demo mode wallpaper update for all devices and remove this feature flag
			chrome.EnableFeatures("DemoModeWallpaperUpdate"),
		)
		if err != nil {
			s.Fatal("Failed to restart Chrome: ", err)
		}
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Fail to get test api conn: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	ui := uiauto.New(tconn).WithTimeout(10 * time.Second)

	if err := uiauto.Combine("open wallpaper subpage",
		personalization.OpenPersonalizationHub(ui),
		personalization.OpenWallpaperSubpage(ui),
	)(ctx); err != nil {
		s.Fatal("Failed to open wallpaper subpage: ", err)
	}

	jupiterText := nodewith.Role(role.Heading).HasClass("preview-text-container").NameStartingWith(fmt.Sprintf("Currently set %s", s.Param()))
	if err := ui.WaitUntilExists(jupiterText)(ctx); err != nil {
		s.Fatal("Failed to validate wallpaper: ", err)
	}
}
