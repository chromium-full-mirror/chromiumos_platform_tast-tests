// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mantis

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/mantis/constant"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/mantis/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/mantis/util"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/galleryapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const imageTestFileName = "a_cake_non_square_20250114.png"

func init() {
	testing.AddTest(&testing.Test{
		Func: ExpandBackgroundPowerMetrics,
		Desc: "Collect power metrics of expand background feature",
		Contacts: []string{
			"cros-mantis@google.com",
			"nurlitadf@google.com",
		},
		BugComponent: "b:1445284",
		Timeout:      constant.PowerTestTimeout,
		SoftwareDeps: []string{"chrome", "chrome_internal", "dlc"},
		HardwareDeps: hwdep.D(hwdep.Model("navi")),
		Data:         []string{imageTestFileName},
		Attr:         []string{"group:crosbolt", "crosbolt_nightly"},
		Fixture:      fixture.PowerAshGaiaWithUpdateEngine,
	})
}

func ExpandBackgroundPowerMetrics(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr := s.FixtValue().(setup.PowerUIFixtureData).Cr
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	if err := util.EnsureDLCInstalled(ctx, constant.MantisDLCID); err != nil {
		s.Fatal("Failed to ensure DLC installed: ", err)
	}

	if err := util.DownloadAndOpenFileInGallery(ctx, cr, s.DataPath(imageTestFileName), imageTestFileName); err != nil {
		s.Fatal("Failed to download and open file in Gallery: ", err)
	}

	ui := uiauto.New(tconn)

	if err := util.OpenEditWithAIPanel(ctx, ui); err != nil {
		s.Fatal("Failed to open edit with AI panel: ", err)
	}

	r := power.NewRecorder(ctx, constant.PowerMetricInterval, s.OutDir(), s.TestName())
	defer r.Close(cleanupCtx)

	// Cool down test device.
	if err := r.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
	}

	expandBackgroundButton := nodewith.Role(role.Button).Name("Expand Background").Ancestor(galleryapp.RootFinder).First()
	if err := uiauto.Combine("Open 'Expand Background' panel",
		ui.WithTimeout(time.Minute).WaitUntilExists(expandBackgroundButton),
		ui.LeftClick(expandBackgroundButton))(ctx); err != nil {
		s.Fatal("Unable to open 'Expand Background' panel: ", err)
	}

	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}

	// Pick 16:9 ratio
	ratioButton := nodewith.Role(role.RadioButton).Name("Ratio Square").Ancestor(galleryapp.RootFinder).First()
	if err := uiauto.Combine("Click ratio option",
		ui.WithTimeout(time.Second*5).WaitUntilExists(ratioButton),
		ui.LeftClick(ratioButton))(ctx); err != nil {
		s.Fatal("Unable to click ratio option: ", err)
	}

	// Click on the expand background button
	if err := uiauto.Combine("Start expand background process",
		ui.WithTimeout(time.Minute).WaitUntilExists(expandBackgroundButton),
		ui.LeftClick(expandBackgroundButton))(ctx); err != nil {
		s.Fatal("Unable to click 'Expand Background' button: ", err)
	}

	s.Log("Expand Background on process")
	if err := util.WaitForSpinner(ctx, tconn, ui); err != nil {
		s.Fatal("Error while waiting for spinner: ", err)
	}

	counter := 0

	startTime := time.Now()
	retryButton := nodewith.Role(role.Button).Name("Retry").Ancestor(galleryapp.RootFinder)
	// Retry for one minute
	for time.Since(startTime) < time.Minute {
		if err := uiauto.Combine("Click the retry button",
			ui.MakeVisible(retryButton),
			ui.LeftClick(retryButton))(ctx); err != nil {
			s.Fatal("Unable to click the retry button: ", err)
		}

		if err := util.WaitForSpinner(ctx, tconn, ui); err != nil {
			s.Fatalf("Error while waiting for spinner during retry %v: %v", counter, err)
		}

		counter++
	}

	s.Logf("Expand Background has been retried %v times", counter)

	doneButton := nodewith.Role(role.Button).Name("Done").Ancestor(galleryapp.RootFinder)
	if err := ui.LeftClick(doneButton)(ctx); err != nil {
		s.Fatal("Failed to click the done button: ", err)
	}

	if err := r.Finish(ctx); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}
}
