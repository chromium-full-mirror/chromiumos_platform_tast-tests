// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hotspot

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/hotspot/hotspotutil"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/services/cros/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
	"google.golang.org/protobuf/types/known/emptypb"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         UINotShownOnNonCellularDevice,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that hotspot UI should not show on non-cellular capable device",
		Contacts: []string{
			"cros-connectivity@google.com",
			"jiajunz@google.com",
		},
		BugComponent: "b:1281224", // ChromeOS > Software > System Services > Connectivity > Hotspot
		Attr:         []string{"group:wificell", "wificell_func", "wificell_unstable"},
		ServiceDeps: []string{
			"tast.cros.browser.ChromeService",
			"tast.cros.chrome.uiauto.ossettings.OsSettingsService",
			"tast.cros.ui.AutomationService",
			"tast.cros.ui.ChromeUIService",
		},
		HardwareDeps: hwdep.D(hwdep.WifiSAP(), hwdep.NoCellular()),
		SoftwareDeps: []string{"chrome"},
		Fixture:      "wificellFixt",
		Params:       []testing.Param{},
	})
}

// UINotShownOnNonCellularDevice tests that hotspot UI should not show on a non-cellular capable device.
func UINotShownOnNonCellularDevice(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	rpcClient := tf.DUTRPC(wificell.DefaultDUT)

	cr := ui.NewChromeServiceClient(rpcClient.Conn)
	defer cr.Close(cleanupCtx, &emptypb.Empty{})

	if _, err := cr.New(ctx, &ui.NewRequest{
		EnableFeatures: []string{"QsRevamp", "Hotspot"},
	}); err != nil {
		s.Fatal("Failed to start Chrome with hotspot flag enabled: ", err)
	}

	settings := ossettings.NewOsSettingsServiceClient(rpcClient.Conn)
	defer settings.Close(cleanupCtx, &emptypb.Empty{})

	if _, err := settings.LaunchAtNetwork(ctx, &emptypb.Empty{}); err != nil {
		s.Fatal("Failed to launch OS-Settings at Network page: ", err)
	}
	defer hotspotutil.DumpUITreeWithScreenshotToFile(cleanupCtx, rpcClient.Conn, s.HasError, "ui_dump")

	uiauto := ui.NewAutomationServiceClient(rpcClient.Conn)
	hotspotFinder := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Name{Name: "hotspot"}},
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_GENERIC_CONTAINER}},
		},
	}
	if _, err := uiauto.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: hotspotFinder}); err == nil {
		s.Fatal("Hotspot summary item should not show on non-cellular capable device")
	}
}
