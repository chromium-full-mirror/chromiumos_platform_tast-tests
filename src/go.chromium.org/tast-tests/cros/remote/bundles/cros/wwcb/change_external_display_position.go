// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wwcb

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/log"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils/topology"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast-tests/cros/services/cros/wwcb"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChangeExternalDisplayPosition,
		Desc:         "Change position of external display relative to DUT",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{
			"group:wwcb",
			"group:pasit",
			"group:release-health",
			"release-health_usb",
		},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"DockingID", "newTestItem"},
		ServiceDeps: []string{"tast.cros.wwcb.DisplayService",
			"tast.cros.browser.ChromeService",
			"tast.cros.ui.ChromeUIService"},
		Data:    []string{"Capabilities.json"},
		Fixture: "wwcb.dock",
		Params: []testing.Param{
			{
				Name:      "fast",
				ExtraAttr: []string{"pasit_fast"},
			}},
	})
}

func ChangeExternalDisplayPosition(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Connect to the gRPC server on the DUT.
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(cleanupCtx)

	// Start Chrome on the DUT.
	cs := ui.NewChromeServiceClient(cl.Conn)
	loginReq := &ui.NewRequest{}
	if _, err := cs.New(ctx, loginReq, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cs.Close(cleanupCtx, &empty.Empty{})

	displaySvc := wwcb.NewDisplayServiceClient(cl.Conn)

	// Dump the UI tree and screenshot on any failure
	utils.AttachErrorHandlersForUITreeDump(cleanupCtx, s, cl.Conn)

	// Connect external display via Dock.
	tf := s.FixtValue().(*topology.TestFixture)
	if _, err := tf.Helper.ActivateDeviceByType(ctx, topology.DeviceTypeMonitor); err != nil {
		s.Fatal("Failed to connect external display: ", err)
	}

	if _, ok := s.Var("newTestItem"); ok {
		if err := tf.VerifyDockingInterface(ctx, s.DUT(), s.DataPath("Capabilities.json")); err != nil {
			s.Fatal("Failed to verify the docking station interface: ", err)
		}
	}

	defer func(ctx context.Context) {
		if s.HasError() {
			log.CollectedLogs(ctx, s.DUT(), s.OutDir())
		}
	}(ctx)

	if _, err := displaySvc.VerifyDisplayCount(ctx, &wwcb.QueryRequest{DisplayCount: 2}); err != nil {
		s.Fatal("Failed to verify external display is connected: ", err)
	}

	if _, err := displaySvc.ChangeRelativePosition(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to change relative position of external display: ", err)
	}
}
