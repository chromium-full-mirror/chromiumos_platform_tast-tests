// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"strings"

	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           ESimNetworkName,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Tests the carrier name on connected esim",
		Contacts: []string{
			"cros-connectivity@google.com",
			"nikhilcn@google.com",
		},
		BugComponent: "b:1131774", // ChromeOS > Software > System Services > Connectivity > Cellular
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:cellular", "cellular_sim_prod_esim"},
		Fixture:      "cellularWithFunctioningSim",
	})
}

func ESimNetworkName(ctx context.Context, s *testing.State) {
	cr, err := chrome.New(ctx)

	helper := s.FixtValue().(*cellular.FixtData).Helper
	if _, err := helper.Connect(ctx); err != nil {
		s.Fatal("Failed to connect to cellular service: ", err)
	}

	networkName, err := helper.GetCurrentNetworkName(ctx)
	if err != nil {
		s.Fatal("Error fetching the current network name: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)

	app, err := ossettings.OpenMobileDataSubpage(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open mobile data subpage: ", err)
	}

	expr := `var optionNode = shadowPiercingQuery('div[id="itemTitle"][aria-hidden="true"]');
	         if (optionNode == undefined) {
		       throw new Error("Title node not found.");
	         }
	         optionNode.innerText;`

	var title string
	if err := app.EvalJSWithShadowPiercer(ctx, cr, expr, &title); err != nil {
		s.Fatal("Failed to fetch title: ", err)
	}

	if !strings.Contains(title, networkName) {
		s.Fatal("Network name is not present in the title: ", networkName)
	}
}
