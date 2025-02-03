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
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type reimagineTestParameters struct {
	withPrompt bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func: ReimaginePowerMetrics,
		Desc: "Collect power metrics of reimagine feature",
		Contacts: []string{
			"cros-mantis@google.com",
			"nurlitadf@google.com",
		},
		BugComponent: "b:1445284",
		Timeout:      constant.PowerTestTimeout,
		SoftwareDeps: []string{"chrome", "chrome_internal", "dlc"},
		HardwareDeps: hwdep.D(hwdep.Model("navi")),
		Data:         []string{constant.ImageTestFileName},
		Attr:         []string{"group:crosbolt", "crosbolt_nightly"},
		Fixture:      fixture.PowerAshGaiaWithUpdateEngine,
		Params: []testing.Param{
			{
				Name: "gen_fill",
				Val: reimagineTestParameters{
					withPrompt: true,
				},
			},
			{
				Name: "inpainting",
				Val: reimagineTestParameters{
					withPrompt: false,
				},
			},
		},
	})
}

func ReimaginePowerMetrics(ctx context.Context, s *testing.State) {
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

	// Set up keyboard.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(ctx)

	if err := util.DownloadAndOpenFileInGallery(ctx, cr, s.DataPath(constant.ImageTestFileName), constant.ImageTestFileName); err != nil {
		s.Fatal("Failed to download and open file in Gallery: ", err)
	}

	ui := uiauto.New(tconn)

	editWithAIButton := nodewith.Role(role.ToggleButton).Name("Edit with AI").Ancestor(galleryapp.RootFinder)
	if err := uiauto.Combine("Trigger Mantis initialization by clicking on 'Edit with AI' button",
		ui.WithTimeout(time.Minute).WaitUntilExists(editWithAIButton),
		ui.LeftClick(editWithAIButton))(ctx); err != nil {
		s.Fatal("Unable to click 'Edit with AI' button: ", err)
	}

	r := power.NewRecorder(ctx, constant.PowerMetricInterval, s.OutDir(), s.TestName())
	defer r.Close(cleanupCtx)

	// Cool down test device.
	if err := r.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
	}

	reimagineButton := nodewith.Role(role.Button).Name("Reimagine").Ancestor(galleryapp.RootFinder).First()
	if err := uiauto.Combine("Verifies Mantis initialization done by clicking 'Reimagine' button",
		ui.WithTimeout(time.Minute).WaitUntilExists(reimagineButton),
		ui.LeftClick(reimagineButton))(ctx); err != nil {
		s.Fatal("Unable to click 'Reimagine' button: ", err)
	}

	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}

	if err := util.WaitForSpinner(ctx, tconn, ui); err != nil {
		s.Log("Error while waiting for spinner: ", err)
	}

	// Draw a scribble on the image
	if err := util.DrawOnImage(ctx, tconn, ui); err != nil {
		s.Fatal("Cannot draw on the image")
	}

	params := s.Param().(reimagineTestParameters)
	if params.withPrompt {
		// Input the text prompt
		reimagineTextArea := nodewith.Role(role.TextField).Name("What do you want to generate in the area?").Ancestor(galleryapp.RootFinder)
		if err := ui.LeftClick(reimagineTextArea)(ctx); err != nil {
			s.Fatal("Failed to click the prompt text area: ", err)
		}

		if err := kb.Type(ctx, "a cute cat"); err != nil {
			s.Fatal("Failed to type the text prompt: ", err)
		}
	}

	// Click on the reimagine button
	if err := uiauto.Combine("Start reimagine process",
		ui.WithTimeout(time.Minute).WaitUntilExists(reimagineButton),
		ui.LeftClick(reimagineButton))(ctx); err != nil {
		s.Fatal("Unable to click 'Reimagine' button: ", err)
	}

	s.Log("Reimagine on process")
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

	s.Logf("Reimagine has been retried %v times", counter)

	doneButton := nodewith.Role(role.Button).Name("Done").Ancestor(galleryapp.RootFinder)
	if err := ui.LeftClick(doneButton)(ctx); err != nil {
		s.Fatal("Failed to click the done button: ", err)
	}

	if err := r.Finish(ctx); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}
}
