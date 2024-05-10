// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"strings"

	"go.chromium.org/tast-tests/cros/common/networkui/netconfigtypes"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           PSimNetworkName,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleInDevelopment,
		Desc:           "Tests the network name on non-connected psim",
		Contacts: []string{
			"alfredyu@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent: "b:1578688", // ChromeOS > External > Cienet > Manual Test Automation > Test stabilization
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_active"},
		Fixture:      "cellularWithChrome",
	})
}

// PSimNetworkName ensures that PSim network is disconnected and then
// verifies that network name is displayed correctly in Settings.
func PSimNetworkName(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*cellular.FixtData).Chrome

	helper := s.FixtValue().(*cellular.FixtData).Helper

	// Wait for any Cellular service to be selected and connected.
	selectedService, err := helper.Device.WaitForSelectedService(ctx, shillconst.DefaultTimeout)
	if err != nil {
		s.Fatal("Error waiting for device selected service: ", err)
	}
	selectedService.WaitForProperty(ctx, shillconst.ServicePropertyIsConnected,
		true, shillconst.DefaultTimeout)

	pSimService, err := helper.FindPSimService(ctx)
	if err != nil {
		s.Fatal("Error finding PSim service: ", err)
	}

	isConnected, err := pSimService.IsConnected(ctx)
	if err != nil {
		s.Fatal("Error getting IsConnected for service: ", err)
	}

	// Make sure that the PSIM network is not connected.
	if isConnected {
		err = pSimService.Disconnect(ctx)
		if err != nil {
			s.Fatal("Error disconnecting from PSIM: ", err)
		}
		pSimService.WaitForProperty(ctx, shillconst.ServicePropertyIsConnected,
			false, shillconst.DefaultTimeout)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create TestAPIConn: ", err)
	}

	networkName, err := pSimService.GetName(ctx)
	if err != nil {
		s.Fatal("Error getting network name: ", err)
	}

	// Check if the PSim network appears disconnected in the network detail page
	app, err := ossettings.OpenNetworkDetailPage(ctx, tconn, cr, networkName, netconfigtypes.Cellular)
	if err != nil {
		s.Fatal("Failed to open mobile network detail subpage: ", err)
	}
	defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree")

	expr := `var optionNode = shadowPiercingQuery(
                 'settings-internet-detail-subpage div#networkState');
	         if (optionNode == undefined) {
		       throw new Error("Title node not found.");
	         }
	         optionNode.innerText;`

	var networkStatusMessage string
	if err := app.EvalJSWithShadowPiercer(ctx, cr, expr, &networkStatusMessage); err != nil {
		s.Fatal("Failed to fetch network status messsage: ", err)
	}

	if networkStatusMessage != "Not Connected" {
		s.Fatal("PSim network UI does not match connection state: ", networkStatusMessage)
	}

	// Check if the network name is displayed correctly in the Mobile data subpage.
	app, err = ossettings.OpenMobileDataSubpage(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open mobile data subpage: ", err)
	}

	expr = `var optionNode = shadowPiercingQuery(
                 'network-list#psimNetworkList div[id="itemTitle"][aria-hidden="true"]');
	         if (optionNode == undefined) {
		       throw new Error("Title node not found.");
	         }
	         optionNode.innerText;`

	var title string
	if err := app.EvalJSWithShadowPiercer(ctx, cr, expr, &title); err != nil {
		s.Fatal("Failed to fetch title: ", err)
	}

	title = strings.TrimSpace(title)
	networkName = strings.TrimSpace(networkName)
	if networkName != title {
		s.Fatalf("Network name is not the same as title. Got %q expected %q", title, networkName)
	}
}
