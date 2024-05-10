// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package apps

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/apps/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/apps/pre"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/helpapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LaunchReleaseNotesFromSettings,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Help app release notes can be launched from Settings",
		Contacts: []string{
			"showoff-eng@google.com",
		},
		BugComponent: "b:690873",
		SoftwareDeps: []string{"chrome", "chrome_internal"},
		Params: []testing.Param{
			{
				Name:              "stable_fieldtrial_testing_config_off",
				ExtraHardwareDeps: hwdep.D(pre.AppsStableModels),
				ExtraAttr:         []string{"group:mainline"},
				Fixture:           fixture.LoggedInFieldTrialConfigDisable,
			}, {
				Name:              "stable_fieldtrial_testing_config_on",
				ExtraHardwareDeps: hwdep.D(pre.AppsStableModels),
				ExtraAttr:         []string{"group:mainline", "group:chrome_uprev_cbx"},
				Fixture:           fixture.LoggedInFieldTrialConfigEnable,
			}, {
				Name:              "unstable",
				ExtraHardwareDeps: hwdep.D(pre.AppsUnstableModels),
				// b:238260020 - disable aged (>1y) unpromoted informational tests
				// ExtraAttr:         []string{"group:mainline", "informational"},
				Fixture: fixture.LoggedIn,
			},
		},
	})
}

// LaunchReleaseNotesFromSettings verifies launching Help app at the release notes page from ChromeOS settings.
func LaunchReleaseNotesFromSettings(ctx context.Context, s *testing.State) {
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	ui := uiauto.New(tconn)

	settings, err := ossettings.LaunchAtPage(ctx, tconn, ossettings.AboutChromeOS)
	if err != nil {
		s.Fatal("Failed to launch Settings: ", err)
	}

	whatsNewTabFinder := nodewith.NameRegex(regexp.MustCompile("(W|w)hat('|’)s (n|N)ew")).Role(role.Heading).Ancestor(helpapp.RootFinder)

	if err := uiauto.Combine("launch WhatsNew and verify landing page",
		settings.LaunchWhatsNew(),
		ui.WaitUntilExists(whatsNewTabFinder),
	)(ctx); err != nil {
		s.Error(`Failed to launch WhatsNew or verify landing page: `, err)
	}
}
