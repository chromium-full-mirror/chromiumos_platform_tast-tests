// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package shimlessrma contains integration tests for Shimless RMA SWA.
package shimlessrma

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/shimlessrma/util"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/shimlessrmaapp"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	scenarioWelcomeCancel                  = "welcome_cancel"
	scenarioWelcomeNextCancel              = "welcome_next_cancel"
	scenarioSelectComponentsNoneNextCancel = "select_components_none_next_cancel"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: CancelFlow,
		Desc: "Verifies different cancellation flows in the Shimless RMA app",
		Contacts: []string{
			"chromeos-shimless-eng@google.com",
			"chenghan@google.com",
			"jeffulin@google.com",
		},
		BugComponent: "b:1002147",
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		HardwareDeps: hwdep.D(hwdep.GSCUART(), hwdep.Battery(), hwdep.MinStorage(16)),
		SoftwareDeps: []string{"chrome"},
		Timeout:      5 * time.Minute,
		Params: []testing.Param{
			{
				Name:              "welcome_cancel_critical",
				Val:               scenarioWelcomeCancel,
				ExtraHardwareDeps: hwdep.D(hwdep.RuntimeProbeConfig()),
				ExtraAttr:         []string{"group:mainline"},
			}, {
				Name:              "welcome_cancel_staging",
				Val:               scenarioWelcomeCancel,
				ExtraHardwareDeps: hwdep.D(hwdep.Model(util.ShimlessRmaEnabledModelsStaging...)),
				ExtraAttr:         []string{"group:shimless_rma", "shimless_rma_normal"},
			}, {
				Name:              "welcome_next_cancel_critical",
				Val:               scenarioWelcomeNextCancel,
				ExtraHardwareDeps: hwdep.D(hwdep.RuntimeProbeConfig()),
				ExtraAttr:         []string{"group:mainline"},
			}, {
				Name:              "welcome_next_cancel_staging",
				Val:               scenarioWelcomeNextCancel,
				ExtraHardwareDeps: hwdep.D(hwdep.Model(util.ShimlessRmaEnabledModelsStaging...)),
				ExtraAttr:         []string{"group:shimless_rma", "shimless_rma_normal"},
			}, {
				Name:              "select_components_none_next_cancel_critical",
				Val:               scenarioSelectComponentsNoneNextCancel,
				ExtraHardwareDeps: hwdep.D(hwdep.RuntimeProbeConfig()),
				ExtraAttr:         []string{"group:mainline"},
			}, {
				Name:              "select_components_none_next_cancel_staging",
				Val:               scenarioSelectComponentsNoneNextCancel,
				ExtraHardwareDeps: hwdep.D(hwdep.Model(util.ShimlessRmaEnabledModelsStaging...)),
				ExtraAttr:         []string{"group:shimless_rma", "shimless_rma_normal"},
			},
		},
	})
}

// CancelFlow verifies different cancellation flows in the Shimless RMA app.
func CancelFlow(ctx context.Context, s *testing.State) {
	scenario := s.Param().(string)

	var initialState string
	switch scenario {
	case scenarioWelcomeCancel, scenarioWelcomeNextCancel:
		initialState = ""
	case scenarioSelectComponentsNoneNextCancel:
		initialState = `{"state_history":[1,2]}`
	default:
		s.Fatalf("Unknown scenario: %q", scenario)
	}

	resources, app, err := util.InitResource(ctx, s.RequiredVar("ui.signinProfileTestExtensionManifestKey"), initialState)
	if err != nil {
		s.Fatalf("Failed to init resource for scenario %q: %v", scenario, err)
	}
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	defer resources.DisposeResource(cleanupCtx)

	act, err := generateCancelAction(scenario, app)
	if err != nil {
		s.Fatalf("Failed to generate action for scenario %q: %v", scenario, err)
	}

	if err := act(ctx); err != nil {
		s.Fatalf("Failed actions for scenario %q: %v", scenario, err)
	}
}

// generateCancelAction returns the action for a given cancellation scenario.
func generateCancelAction(scenario string, app *shimlessrmaapp.RMAApp) (action.Action, error) {
	switch scenario {
	case scenarioWelcomeCancel:
		return action.Combine("Welcome and cancel",
			app.WaitForPageToLoad("Chromebook repair", 20*time.Second),
			app.LeftClickButton("Exit")), nil
	case scenarioWelcomeNextCancel:
		return action.Combine("Welcome and then next and then cancel",
			app.WaitForPageToLoad("Chromebook repair", 20*time.Second),
			app.WaitUntilButtonEnabled("Get started", 30*time.Second),
			app.LeftClickButton("Get started"),
			app.WaitForPageToLoad("Select which components were replaced", 30*time.Second),
			app.LeftClickButton("Exit")), nil
	case scenarioSelectComponentsNoneNextCancel:
		return action.Combine("Select Component none and then next and then cancel",
			app.WaitForPageToLoad("Select which components were replaced", 30*time.Second),
			app.LeftClickButton("Next"),
			app.WaitForPageToLoad("After repair, who will be using the device?", 20*time.Second),
			app.LeftClickButton("Exit")), nil
	}
	return nil, errors.Errorf("unknown scenario: %q", scenario)
}
