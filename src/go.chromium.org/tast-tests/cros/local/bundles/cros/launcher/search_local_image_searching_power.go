// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package launcher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SearchLocalImageSearchingPower,
		LacrosStatus: testing.LacrosVariantUnneeded, // Image search won't interact with Chrome browser.
		Desc:         "Checks launcher image search power usage",
		Contacts: []string{
			"ml-service-team@google.com",
			"xiuwen@google.org",
		},
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		Data:         []string{launcher.ImageSearchPowerTestPictureName},
		BugComponent: "b:1281467",
		Timeout:      20*time.Minute + power.RecorderTimeout,
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{
			{
				Name:    "enable",
				Fixture: setup.PowerImageSearchWithFlagOn,
			},
			{
				Name:    "disable",
				Fixture: setup.PowerAsh,
			},
		},
	})
}

func SearchLocalImageSearchingPower(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(setup.PowerUIFixtureData).Cr

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	// Preparation for launcher test.
	cleanup, err := launcher.SetUpLauncherTest(ctx, tconn, false /*tabletMode*/, false /*stabilizeAppCount*/)
	if err != nil {
		s.Fatal("Failed to set up launcher test case: ", err)
	}
	defer cleanup(cleanupCtx)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}

	// Get file base path.
	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user's Download path: ", err)
	}

	localFileLocation := filepath.Join(downloadsPath, launcher.ImageSearchPowerTestPictureName)

	// Ensure all required DLCs are installed.
	if err := launcher.InstallDlc(ctx, []string{"screen-ai", "ml-core-internal"}); err != nil {
		s.Fatal("Cannot install dlc: ", err)
	}

	// Prepare images.
	for i := 0; i < launcher.ImageSearchPowerTestRepeatTimes; i++ {
		imageFile := localFileLocation + fmt.Sprintf("%d.png", i)
		if err := fsutil.CopyFile(s.DataPath(launcher.ImageSearchPowerTestPictureName), imageFile); err != nil {
			s.Fatalf("Failed to copy the test image to %s: %v", localFileLocation, err)
		}
		defer os.Remove(imageFile)
	}

	// Start power test for image search.
	r := power.NewRecorder(ctx, 5*time.Second, s.OutDir(), s.TestName())
	defer r.Close(cleanupCtx)
	if err := r.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
	}
	if err := r.Start(ctx); err != nil {
		s.Fatal("Failed to start collecting power metrics: ", err)
	}

	// Search content in the test image.
	query := "Paper"
	for i := 0; i < launcher.ImageSearchPowerTestRepeatTimes; i++ {
		if err := uiauto.NamedCombine("Search for image",
			launcher.ClearSearchField(tconn, kb),
			launcher.Search(tconn, kb, query),
		)(ctx); err != nil {
			s.Fatal("Failed to search image: ", err)
		}

		//GoBigSleepLint: Need enough time for UI to fully showup
		testing.Sleep(ctx, 1*time.Second)
	}

	// Stop power test and clean the environment.
	if err := r.Finish(ctx); err != nil {
		s.Error("Failed to finish collecting power metrics: ", err)
	}
}
