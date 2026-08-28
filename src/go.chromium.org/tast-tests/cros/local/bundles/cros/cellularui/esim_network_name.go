// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellularui

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           ESimNetworkName,
		LifeCycleStage: testing.LifeCycleInDevelopment,
		Desc:           "Tests the carrier name on connected esim",
		Contacts: []string{
			"alfredyu@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent: "b:1578688", // ChromeOS > External > Cienet > Manual Test Automation > Test stabilization
		SoftwareDeps: []string{"chrome"},
		// TODO(b/542029943): Re-enable once the test is stable.
		// Attr:         []string{"group:cellular", "cellular_sim_prod_esim"},
		Fixture: "cellularWithFunctioningSim",
	})
}

func ESimNetworkName(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to create a new instance of Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	helper := s.FixtValue().(*cellular.FixtData).Helper
	if _, err := helper.Connect(ctx); err != nil {
		s.Fatal("Failed to connect to cellular service: ", err)
	}

	networkName, err := helper.GetCurrentNetworkName(ctx)
	if err != nil {
		s.Fatal("Error fetching the current network name: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	app, err := ossettings.LaunchAtMobileData(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open mobile data subpage: ", err)
	}
	defer app.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotWithTestAPIOnError(cleanupCtx, s.OutDir(), s.HasError, tconn, "os_settings")

	if err := ossettings.WaitUntilRefreshCellularProfileCompletes(ctx, tconn); err != nil {
		s.Fatal("Failed to wait until refresh profile complete: ", err)
	}

	const expr string = `var optionNode = shadowPiercingQuery('div[id="itemTitle"][aria-hidden="true"]');
	         if (optionNode == undefined) {
		       throw new Error("Title node not found.");
	         }
	         optionNode.innerText;`

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var title string
		if err := app.EvalJSWithShadowPiercer(ctx, cr, expr, &title); err != nil {
			return errors.Wrap(err, "failed to fetch title")
		}

		if !strings.Contains(title, networkName) {
			return errors.Wrapf(err, "network name %q is not present in the title: %q", networkName, title)
		}

		return nil
	}, &testing.PollOptions{Timeout: 15 * time.Second, Interval: time.Second}); err != nil {
		s.Fatal("Failed to wait until the carrier name shown with the connected esim: ", err)
	}
}
