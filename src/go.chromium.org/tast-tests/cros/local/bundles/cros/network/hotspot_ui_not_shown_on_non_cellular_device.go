// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           HotspotUINotShownOnNonCellularDevice,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Tests that hotspot UI should not show on non-cellular capable devices",
		Contacts: []string{
			"cros-connectivity@google.com",
			"jiajunz@google.com",
		},
		BugComponent: "b:1281224", // ChromeOS > Software > System Services > Connectivity > Hotspot
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.WifiSAP(), hwdep.NoCellular()),
		Attr:         []string{"group:wificell", "wificell_e2e_unstable"},
	})
}

func HotspotUINotShownOnNonCellularDevice(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr, err := chrome.New(ctx, chrome.EnableFeatures("Hotspot"))
	if err != nil {
		s.Fatal("Failed to create new chrome instance: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	if _, err := ossettings.LaunchAtPage(ctx, tconn, ossettings.Network); err != nil {
		s.Fatal("Failed to launch Network page: ", err)
	}
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	ui := uiauto.New(tconn)

	if found, err := ui.IsNodeFound(ctx, nodewith.NameContaining("Hotspot")); err != nil {
		s.Fatal("Error while looking for Hotspot summary item: ", err)
	} else if found {
		s.Fatal("Hotspot summary item should not be shown on non-cellular capable device")
	}
}
