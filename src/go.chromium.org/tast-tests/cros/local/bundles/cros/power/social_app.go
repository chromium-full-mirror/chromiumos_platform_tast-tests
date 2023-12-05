// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/power/socialapp"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

// Experiment data from b/308846863#comment5
const (
	elementPrepareTimeout        = 10 * time.Minute
	socialAppMeasurementInterval = 20 * time.Second
	socialAppOperatingTimeout    = 30 * time.Minute
	socialAppTimeout             = elementPrepareTimeout + socialAppOperatingTimeout + power.RecorderTimeout
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SocialApp,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Collect the power-related data for social app operations",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"cienet-development@googlegroups.com",
			"jason.hsiao@cienet.com",
		},
		BugComponent: "b:1361410", // ChromeOS > Platform > System > Core Power
		SoftwareDeps: []string{"chrome", "arc"},
		Vars:         socialapp.ElementApkURLVars, // Optional. The URL of the APK file of Element app.
		Timeout:      socialAppTimeout,
		Params: []testing.Param{{
			Name:      "element_ash",
			Fixture:   "powerAshARC",
			ExtraAttr: []string{"group:power", "power_regression"},
		}, {
			Name:    "element_lacros",
			Fixture: "powerLacrosARC",
		}},
	})
}

// SocialApp operates the social apps and collects the power-related data.
func SocialApp(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(setup.PowerUIFixtureData).Cr
	a := s.FixtValue().(setup.PowerUIFixtureData).ARC

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API connection: ", err)
	}

	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	d, err := a.NewUIDevice(ctx)
	if err != nil {
		s.Fatal("Failed to set up ARC and Play Store: ", err)
	}
	defer func(ctx context.Context) {
		// If an error happened, the device will be closed before dumping ARC UI.
		// Only close if the device is alive.
		if d.Alive(ctx) {
			d.Close(ctx)
		}
	}(closeCtx)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to initialize keyboard: ", err)
	}
	defer kb.Close(closeCtx)

	splitAccount := strings.Split(cr.Creds().User, "@")
	if len(splitAccount) < 1 {
		s.Fatal("Failed to get username from the account: ", cr.Creds().User)
	}
	username := splitAccount[0]

	apkURL, err := socialapp.ParseElementAPKURL(ctx, s.Var)
	if err != nil {
		// If the DUT failed to parse the APK URL, the returned |apkURL| would be empty.
		// Log the error and try to continue running the case, the DUT would try to install
		// the app from Play Store in this case.
		s.Log("Failed to parse Element APK URL: ", err)
	}
	app := socialapp.NewElement(tconn, kb, a, d, username, apkURL)

	bt := s.FixtValue().(setup.PowerUIFixtureData).Bt
	if bt == browser.TypeLacros {
		// The Element app uses a web page to log in with a Google account.
		// Launch the lacros browser to allow the app to open the web page.
		l, err := lacros.Launch(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to launch lacros: ", err)
		}
		defer l.Close(closeCtx)
	}

	if err := app.Install(ctx); err != nil {
		s.Fatal("Failed to install app: ", err)
	}
	defer app.Uninstall(closeCtx)

	recorder := power.NewRecorder(ctx, socialAppMeasurementInterval, s.OutDir(), s.TestName())
	defer recorder.Close(closeCtx)

	if err := recorder.Cooldown(ctx); err != nil {
		s.Error("Failed to cool down the device: ", err)
	}

	prepareStartTime := time.Now()
	if err := app.Launch(ctx); err != nil {
		s.Fatal("Failed to launch app: ", err)
	}

	isAppSetup := false
	defer func(ctx context.Context) {
		// Closing UI device might cause the cleanup to fail.
		// Only close the UI device when an error occurred.
		if s.HasError() {
			// Close the ARC UI device before dumping ARC UI Hierarchy.
			// Otherwise uiautomator might exist with errors.
			if err := d.Close(ctx); err != nil {
				s.Log("Failed to close the ARC UI device: ", err)
			}
			a.DumpUIHierarchyOnError(ctx, filepath.Join(s.OutDir(), "arc"), s.HasError)
			faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_dump")
		}
		if isAppSetup {
			if err := app.CleanUp(ctx); err != nil {
				s.Log("Failed to cleanup app: ", err)
			}
		}
		if err := app.Close(ctx); err != nil {
			s.Log("Failed to close the app: ", err)
		}
	}(closeCtx)

	if err := app.SetUp(ctx); err != nil {
		s.Fatal("Failed to setup app for testing: ", err)
	}
	isAppSetup = true

	s.Log("Launching and setting up the app took ", time.Since(prepareStartTime))

	if err := recorder.Start(ctx); err != nil {
		s.Fatal("Failed to start collecting power metrics: ", err)
	}

	// Keep operating the app for |socialAppOperatingTimeout| to collect the power-related data.
	i := 1
	for endTime := time.Now().Add(socialAppOperatingTimeout); time.Now().Before(endTime); {
		if err := uiauto.NamedCombine(fmt.Sprintf("run test scenarios %d times", i),
			app.SendMessages,
			app.RunExtraOperations,
		)(ctx); err != nil {
			s.Fatal("Failed to run the test scenario: ", err)
		}
		i++
	}

	if err := recorder.Finish(ctx); err != nil {
		s.Fatal("Failed to finish collecting power metrics: ", err)
	}
}
