// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package apps

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/apps/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/apps/pre"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/helpapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LaunchHelpAppFromLauncher,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Help app can be found and launched from the launcher",
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
				// TODO(b/321306051): Promote to critical.
				ExtraAttr: []string{"group:mainline", "informational", "group:criticalstaging"},
				Fixture:   fixture.LoggedInFieldTrialConfigEnable,
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

// LaunchHelpAppFromLauncher verifies launching Help app from the launcher.
func LaunchHelpAppFromLauncher(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(ctx)
	if err := launcher.SearchAndWaitForAppOpen(tconn, kb, apps.Help)(ctx); err != nil {
		s.Fatal("Failed to launch help app: ", err)
	}

	// App should be launched at the overview page.
	if err := helpapp.NewContext(cr, tconn).WaitForApp()(ctx); err != nil {
		s.Fatal("Failed to wait for help app: ", err)
	}
}
