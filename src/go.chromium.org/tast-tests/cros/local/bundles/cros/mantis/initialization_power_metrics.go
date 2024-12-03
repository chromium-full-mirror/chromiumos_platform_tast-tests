// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mantis

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/mantis/util"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/galleryapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/dlc"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast-tests/cros/local/updateengine"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	testFile            = "a_horse_20241127.png"
	powerMetricInterval = 5 * time.Second
	mantisDLCID         = "ml-dlc-302a455f-5453-43fb-a6a1-d856e6fe6435"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: InitializationPowerMetrics,
		Desc: "Collect power metrics of mantis initialization",
		Contacts: []string{
			"cros-mantis@google.com",
			"nurlitadf@google.com",
		},
		BugComponent: "b:1445284",
		// DLC download might take up to 20 minutes
		Timeout:      20 * time.Minute,
		SoftwareDeps: []string{"chrome", "chrome_internal", "dlc"},
		HardwareDeps: hwdep.D(hwdep.Model("navi")),
		Data:         []string{testFile},
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		Fixture:      setup.PowerAshGAIA,
	})
}

func InitializationPowerMetrics(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr := s.FixtValue().(setup.PowerUIFixtureData).Cr
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	r := power.NewRecorder(ctx, powerMetricInterval, s.OutDir(), s.TestName())
	defer r.Close(cleanupCtx)

	// Cool down test device.
	if err := r.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
	}

	// Ensure that the update engine service is ready to receive DLC install request from DLC service.
	if err := upstart.RestartJobAndWaitForDbusService(ctx, updateengine.JobName, updateengine.ServiceName); err != nil {
		s.Fatalf("Failed to ensure %s running: %v", updateengine.JobName, err)
	}

	// Check dlcservice is up and running.
	if err := upstart.EnsureJobRunning(ctx, dlc.JobName); err != nil {
		s.Fatalf("Failed to ensure %s running: %v", dlc.JobName, err)
	}

	// Install DLC.
	if err := dlc.Install(ctx, mantisDLCID, ""); err != nil {
		s.Fatal("Failed to install DLC: ", err)
	}

	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}

	if err := util.DownloadAndOpenFileInGallery(ctx, cr, s.DataPath(testFile), testFile); err != nil {
		s.Fatal("Failed to download and open file in Gallery: ", err)
	}

	ui := uiauto.New(tconn)

	editWithAIButton := nodewith.Role(role.ToggleButton).Name("Edit with AI").Ancestor(galleryapp.RootFinder)
	if err := uiauto.Combine("Trigger Mantis initialization by clicking on 'Edit with AI' button",
		ui.WithTimeout(time.Minute).WaitUntilExists(editWithAIButton),
		ui.LeftClick(editWithAIButton))(ctx); err != nil {
		s.Fatal("Unable to click 'Edit with AI' button: ", err)
	}

	reimagineButton := nodewith.Role(role.Button).Name("Reimagine").Ancestor(galleryapp.RootFinder).First()
	if err := uiauto.Combine("Verifies Mantis initialization done by clicking 'Reimagine' button",
		ui.WithTimeout(time.Minute).WaitUntilExists(reimagineButton),
		ui.LeftClick(reimagineButton))(ctx); err != nil {
		s.Fatal("Unable to click 'Reimagine' button: ", err)
	}

	s.Log("Let the device idle")
	// GoBigSleepLint: sleep to let the device idle.
	if err := testing.Sleep(ctx, time.Minute); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	if err := r.Finish(ctx); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}
}
