// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/power/arcvideoplayback"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/power/multitaskingapp"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/power/socialapp"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	// Execution time ratio of browser, social app and video app is 2:3:5.
	// browserWaitTime is set to 12 seconds for each page.
	// The total execution time including swipe up and down is about 2 minutes.
	// socialAppTime is set to 3 minutes.
	// videoPlayTime is set to 5 minutes.
	socialAppTime = 3 * time.Minute
	videoPlayTime = 5 * time.Minute
	browserTime   = 2 * time.Minute

	multiTaskingAppPrepareTimeout = 10 * time.Minute
	multiTaskingAppExecutionTime  = socialAppTime + videoPlayTime + browserTime
	multiTaskingAppShortTimeout   = multiTaskingAppPrepareTimeout + multiTaskingAppExecutionTime*2 + power.RecorderTimeout
	multiTaskingAppTimeout        = multiTaskingAppPrepareTimeout + multiTaskingAppExecutionTime*3*2 + power.RecorderTimeout
)

type multiTaskingParam struct {
	socialAppTime time.Duration
	videoPlayTime time.Duration
	browserTime   time.Duration
}

var multiTaskingAppShortParam = multiTaskingParam{
	socialAppTime: socialAppTime,
	videoPlayTime: videoPlayTime,
	browserTime:   browserTime,
}

var multiTaskingAppParam = multiTaskingParam{
	socialAppTime: socialAppTime * 3,
	videoPlayTime: videoPlayTime * 3,
	browserTime:   browserTime * 3,
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         MultiTaskingApp,
		Desc:         "Collect power related data when device do multi tasking with several apps",
		Contacts:     []string{"chromeos-power-team@google.com"},
		BugComponent: "b:1361410", // ChromeOS > Platform > System > Core Power
		// Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{"chrome", "arc"},
		HardwareDeps: hwdep.D(hwdep.MinStorage(17)), // 16Gb devices may not have enough space to install all the testing apps.
		Data:         []string{multitaskingapp.VideoSrc, arcvideoplayback.ExoPlayerAPKFileName},
		Vars:         socialapp.ElementApkURLVars, // Optional. The URL of the APK file of Element app.
		Fixture:      setup.PowerAshARCWithTuwunelServer,
		Params: []testing.Param{
			{
				Name:    "ash",
				Timeout: multiTaskingAppTimeout + setup.BatteryPreparationTimeout,
				ExtraAttr: []string{
					"group:power",
					"power_regression",
					"group:release-health",
					"release-health_power",
				},
				Val: multiTaskingAppParam,
			},
			{
				Name:    "ash_short",
				Timeout: multiTaskingAppShortTimeout + setup.BatteryPreparationTimeout,
				ExtraAttr: []string{
					"group:power",
					"power_daily",
					"group:release-health",
					"release-health_power",
				},
				Val: multiTaskingAppShortParam,
			},
		},
	})
}

// MultiTaskingApp collects power related data when device do multi tasking with several apps.
func MultiTaskingApp(ctx context.Context, s *testing.State) {
	discharge := s.FixtValue().(setup.PowerUIFixtureData).Discharge
	cr := s.FixtValue().(setup.PowerUIFixtureData).Cr
	a := s.FixtValue().(setup.PowerUIFixtureData).ARC
	param := s.Param().(multiTaskingParam)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to open the keyboard: ", err)
	}
	defer kb.Close(cleanupCtx)

	tabletMode, err := ash.TabletModeEnabled(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get tablet mode: ", err)
	}

	var uiHandler cuj.UIActionHandler
	if tabletMode {
		if uiHandler, err = cuj.NewTabletActionHandler(ctx, cr); err != nil {
			s.Fatal("Failed to create tablet action handler: ", err)
		}
	} else {
		if uiHandler, err = cuj.NewClamshellActionHandler(ctx, cr); err != nil {
			s.Fatal("Failed to create clamshell action handler: ", err)
		}
	}

	socialAppTime := s.Param().(multiTaskingParam).socialAppTime
	videoPlayTime := s.Param().(multiTaskingParam).videoPlayTime
	browserTime := s.Param().(multiTaskingParam).browserTime
	total := socialAppTime + videoPlayTime + browserTime
	batteryCleanup, err := setup.Battery(ctx, total, discharge)
	if err != nil {
		s.Fatal("Setup battery failed: ", err)
	}
	defer batteryCleanup(cleanupCtx)

	testResources := &multitaskingapp.TestResources{
		Cr:        cr,
		Kb:        kb,
		UIHandler: uiHandler,
		Tconn:     tconn,
		A:         a,
	}

	elementAPKURL, err := socialapp.ParseElementAPKURL(ctx, s.Var)
	if err != nil {
		// If the DUT failed to parse the APK URL, the returned |apkURL| would be empty.
		// Log the error and try to continue running the case, the DUT would try to install
		// the app from Play Store in this case.
		s.Log("Failed to parse Element APK URL: ", err)
	}

	params := &multitaskingapp.TestParams{
		OutDir:        s.OutDir(),
		WebSource:     cuj.GoogleWebSource,
		TestName:      s.TestName(),
		ElementAPKURL: elementAPKURL,
		DataPath:      s.DataPath,
		Discharge:     discharge,
		TabletMode:    tabletMode,
		BrowserTime:   param.browserTime,
		SocialAppTime: param.socialAppTime,
		VideoAppTime:  param.videoPlayTime,
	}

	if err := multitaskingapp.Run(ctx, testResources, params); err != nil {
		s.Fatal("Failed to run multi-tasking app test: ", err)
	}
}
