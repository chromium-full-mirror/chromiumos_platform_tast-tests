// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/power/multitaskingapp"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/power/socialapp"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

const (
	// Execution time ratio of browser, social app and video app is 2:3:5.
	// browserWaitTime is set to 12 seconds and executed 5 times in total.
	// The total execution time including swipe up and down is about 2 minutes.
	// socialAppTime is set to 3 minutes.
	// videoPlayTime is set to 5 minutes.
	socialAppTime = 3 * time.Minute
	videoPlayTime = 5 * time.Minute
	browserTime   = 2 * time.Minute

	multiTaskingAppPrepareTimeout = 10 * time.Minute
	multiTaskingAppExecutionTime  = (socialAppTime + videoPlayTime + browserTime) * 2
	multiTaskingAppTimeout        = multiTaskingAppPrepareTimeout + multiTaskingAppExecutionTime + power.RecorderTimeout
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MultiTaskingApp,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Collect power related data when device do multi tasking with several apps",
		Contacts:     []string{"chromeos-platform-power@google.com"},
		BugComponent: "b:1361410", // ChromeOS > Platform > System > Core Power
		// Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{"chrome", "arc"},
		Data:         []string{multitaskingapp.VideoSrc},
		Vars:         socialapp.ElementApkURLVars, // Optional. The URL of the APK file of Element app.
		Timeout:      multiTaskingAppTimeout,
		Params: []testing.Param{
			{
				Name:    "ash",
				Fixture: "powerAshARC",
			},
			{
				Name:    "lacros",
				Fixture: "powerLacrosARC",
			},
		},
	})
}

// MultiTaskingApp collects power related data when device do multi tasking with several apps.
func MultiTaskingApp(ctx context.Context, s *testing.State) {
	bt := s.FixtValue().(setup.PowerUIFixtureData).Bt
	cr := s.FixtValue().(setup.PowerUIFixtureData).Cr
	a := s.FixtValue().(setup.PowerUIFixtureData).ARC

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

	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, bt)
	if err != nil {
		s.Fatal("Failed to setup browser: ", err)
	}
	defer closeBrowser(cleanupCtx)

	bTconn, err := br.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to the TestAPIConn: ", err)
	}

	tabletMode, err := ash.TabletModeEnabled(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get tablet mode: ", err)
	}

	var uiHandler cuj.UIActionHandler
	if tabletMode {
		if uiHandler, err = cuj.NewTabletActionHandler(ctx, tconn); err != nil {
			s.Fatal("Failed to create tablet action handler: ", err)
		}
	} else {
		if uiHandler, err = cuj.NewClamshellActionHandler(ctx, tconn); err != nil {
			s.Fatal("Failed to create clamshell action handler: ", err)
		}
	}

	testResources := &multitaskingapp.TestResources{
		Cr:        cr,
		Kb:        kb,
		UIHandler: uiHandler,
		Tconn:     tconn,
		Btconn:    bTconn,
		A:         a,
		Br:        br,
	}

	elementAPKURL, err := socialapp.ParseElementAPKURL(ctx, s.Var)
	if err != nil {
		// If the DUT failed to parse the APK URL, the returned |apkURL| would be empty.
		// Log the error and try to continue running the case, the DUT would try to install
		// the app from Play Store in this case.
		s.Log("Failed to parse Element APK URL: ", err)
	}

	videoPath := s.DataPath(multitaskingapp.VideoSrc)
	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to retrieve user's Downloads path: ", err)
	}
	targetFilePath := filepath.Join(downloadsPath, filepath.Base(videoPath))
	if err := fsutil.CopyFile(videoPath, targetFilePath); err != nil {
		s.Fatal("Failed to copy the file to the 'Downloads' folder: ", err)
	}
	defer os.Remove(targetFilePath)

	params := &multitaskingapp.TestParams{
		BrowserType:   bt,
		OutDir:        s.OutDir(),
		WebSource:     cuj.GoogleWebSource,
		TestName:      s.TestName(),
		ElementAPKURL: elementAPKURL,
		TabletMode:    tabletMode,
		BrowserTime:   browserTime,
		SocialAppTime: socialAppTime,
		VideoAppTime:  videoPlayTime,
	}

	if err := multitaskingapp.Run(ctx, testResources, params); err != nil {
		s.Fatal("Failed to run multi-tasking app test: ", err)
	}
}
