// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package inputs will have tast tests for input-related features on Chromebooks.
package inputs

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/inputs/fixture"
	"go.chromium.org/tast-tests/cros/local/inputs/testserver"
	"go.chromium.org/tast-tests/cros/local/inputs/util"
	"go.chromium.org/tast-tests/cros/local/perfutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// internalUIPerfRun is used by this file to run the performance scenario.
// It's a copy of the code in
// go.chromium.org/tast-tests/cros/local/bundles/cros/ui/perf
// since this private file cannot depend non-private bundles.
func internalUIPerfRun(s *testing.State, scenario perfutil.ScenarioFunc) func(ctx context.Context, name string) ([]*metrics.Histogram, error) {
	return func(ctx context.Context, name string) ([]*metrics.Histogram, error) {
		var hists []*metrics.Histogram
		var err error
		s.Run(ctx, name, func(ctx context.Context, s *testing.State) {
			hists, err = scenario(ctx, name)
			if err != nil {
				testing.ContextLog(ctx, "Failed to run the test scenario: ", err)
			}
		})
		return hists, err
	}
}

// quickInsertPerfModels is list of models to run the test on.
var quickInsertPerfModels = []string{
	// Common ARM-64
	"burnet",
	// Launch device
	"skolas",
	// Common (and slowest) board is Octopus. Use a couple of popular ones.
	"phaser",
	"sparky",
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         QuickInsertCujPerf,
		Desc:         "Measures the performance of Quick Insert CUJs",
		Contacts:     []string{"essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		Attr:         []string{"group:input-tools"},
		SoftwareDeps: []string{"inputs_deps", "chrome", "chrome_internal"},
		HardwareDeps: hwdep.D(hwdep.Model(quickInsertPerfModels...)),
		SearchFlags:  util.IMESearchFlags([]ime.InputMethod{ime.DefaultInputMethod}),
		Timeout:      2 * time.Minute,
		Fixture:      fixture.ClamshellNonVKWithPicker,
	})
}

func QuickInsertCujPerf(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to access keyboard: ", err)
	}
	defer keyboard.Close(ctx)

	its, err := testserver.LaunchBrowser(ctx, cr, tconn)
	if err != nil {
		s.Fatal("Failed to launch inputs test server: ", err)
	}
	defer its.CloseAll(cleanupCtx)

	ui := uiauto.New(tconn)

	inputField := testserver.TextAreaInputField
	quickInsertWindow := nodewith.HasClass("Quick Insert").Visible().Onscreen()

	runner := perfutil.NewRunner(cr, perfutil.RunnerOptions{IgnoreFirstRun: true, DropMinMaxValues: true})
	runner.RunMultiple(ctx, "", internalUIPerfRun(s, perfutil.RunAndWaitAll(tconn, action.Combine("search emoji and insert",
		its.ClickField(inputField),
		keyboard.AccelAction("Search+F"),
		ui.WaitUntilExists(quickInsertWindow),
		keyboard.TypeAction("thumbs up"),
		ui.LeftClick(nodewith.Ancestor(quickInsertWindow).Name("👍").Role(role.Button).Visible().Onscreen().First()),
		ui.WaitUntilGone(quickInsertWindow),
	),
		"Ash.Picker.Session.InputReadyLatency",
		"Ash.Picker.Session.SearchLatency",
		"Ash.Picker.Session.PresentationLatency.SearchField",
		"Ash.Picker.Session.PresentationLatency.SearchResults")),
		perfutil.StoreLatency)

	if err := runner.Values().Save(ctx, s.OutDir()); err != nil {
		s.Fatal("Failed saving perf data: ", err)
	}
}
