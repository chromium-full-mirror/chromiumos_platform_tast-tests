// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellularui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type testParameters struct {
	roamingSubLabel string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:           RoamingStatusLabel,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Checks the roaming label status on a roaming and non roaming SIM",
		Contacts: []string{
			"chromeos-connectivity-cienet-external@google.com",
			"alfred.yu@cienet.com",
		},
		BugComponent: "b:1578688", // ChromeOS > External > Cienet > Manual Test Automation > Test stabilization
		SoftwareDeps: []string{"chrome"},
		// TODO(b/542029943): Re-enable once the test is stable.
		// Attr:         []string{"group:cellular", "group:release-health", "release-health_cellular"},
		Params: []testing.Param{
			{
				Name: "on_roaming_sim",
				// ExtraAttr: []string{"cellular_sim_roaming"},
				Val: testParameters{
					roamingSubLabel: "Currently roaming",
				},
				Fixture: "cellularWithFunctioningRoamingSim",
			},
			{
				Name: "on_non_roaming_sim",
				// ExtraAttr: []string{"cellular_sim_prod_esim"},
				Val: testParameters{
					roamingSubLabel: "Not currently roaming",
				},
				Fixture: "cellularWithFunctioningSim",
			},
		},
		Timeout: 3 * time.Minute,
	})
}

func RoamingStatusLabel(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	cleanup, err := cellular.SetRoamingPolicy(ctx, true, false)
	if err != nil {
		s.Fatal("Failed to set roaming property: ", err)
	}
	defer cleanup(cleanupCtx)

	helper := s.FixtValue().(*cellular.FixtData).Helper
	if _, err := helper.Connect(ctx); err != nil {
		s.Fatal("Failed to connect to cellular service: ", err)
	}

	app, err := ossettings.OpenMobileDataSubpage(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open mobile data sub page: ", err)
	}
	defer app.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotWithTestAPIOnError(cleanupCtx, s.OutDir(), s.HasError, tconn, "ui_dump")

	if err := ossettings.GoToActiveNetworkDetails(ctx, tconn); err != nil {
		s.Fatal("Failed to go to active cellular network detail page view: ", err)
	}

	roamingSubLabel, err := app.RoamingSubLabel(ctx, cr)
	if err != nil {
		s.Fatal("Failed to fetch sublabel: ", err)
	}

	if roamingSubLabel != s.Param().(testParameters).roamingSubLabel {
		s.Fatalf("Roaming sub-label is incorrect: got %q, want %q", roamingSubLabel, s.Param().(testParameters).roamingSubLabel)
	}
}
