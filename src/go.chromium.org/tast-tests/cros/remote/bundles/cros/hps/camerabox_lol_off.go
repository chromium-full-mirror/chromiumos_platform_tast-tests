// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hps

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	wrappers "github.com/golang/protobuf/ptypes/wrappers"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/common/hps/hpsutil"
	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/hps/utils"
	pb "go.chromium.org/tast-tests/cros/services/cros/hps"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CameraboxLoLOff,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that HPS does not dim the screen quickly when LoL is off",
		Data:         []string{hpsutil.PersonPresentPageArchiveFilename},
		Contacts: []string{
			"chromeos-hps-swe@google.com",
			"eunicesun@google.com",
		},
		BugComponent: "b:1140302",
		Attr:         []string{"group:camerabox", "group:hps", "hps_perbuild"},
		Timeout:      10 * time.Minute,
		HardwareDeps: hwdep.D(hwdep.HPS()),
		SoftwareDeps: []string{"hps", "chrome", caps.BuiltinCamera},
		ServiceDeps:  []string{"tast.cros.browser.ChromeService", "tast.cros.hps.HpsService"},
		Vars:         []string{"chart"},
	})
}

func CameraboxLoLOff(ctx context.Context, s *testing.State) {

	dut := s.DUT()

	// Creating hps context.
	hctx, err := hpsutil.NewHpsContext(ctx, "", hpsutil.DeviceTypeBuiltin, s.OutDir(), dut.Conn())
	if err != nil {
		s.Fatal("Error creating HpsContext: ", err)
	}

	// Connecting to the chart tablet that will render the picture.
	ctxForCleanupDisplayChart := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	hostPaths, displayChart, err := utils.SetupDisplay(ctx, s)
	if err != nil {
		s.Fatal("Error setting up display: ", err)
	}
	defer displayChart.Close(ctxForCleanupDisplayChart, s.OutDir())

	displayChart.Display(ctx, hostPaths[utils.ZeroPresence])

	// Connecting to Taeko.
	cleanupCtx := ctx
	ctx, cancel = ctxutil.Shorten(ctx, time.Minute)
	defer cancel()
	cl, err := rpc.Dial(ctx, dut, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to setup grpc: ", err)
	}
	defer cl.Close(cleanupCtx)

	// First turn on the "Lock on leave" feature.
	// This is only needed so that we can scrape the quick dim delay from the powerd logs.
	// We turn it off again below, when performing the actual test.
	client := pb.NewHpsServiceClient(cl.Conn)
	req := &pb.StartUIWithCustomScreenPrivacySettingRequest{
		Setting: utils.LockOnLeave,
		Enable:  true,
	}
	if _, err := client.StartUIWithCustomScreenPrivacySetting(hctx.Ctx, req, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to change setting: ", err)
	}

	// Get the delays for the quick dim.
	delayReq := &wrappers.BoolValue{
		Value: true,
	}
	quickDimMetrics, err := client.RetrieveDimMetrics(hctx.Ctx, delayReq)
	if err != nil {
		s.Fatal("Error getting delay settings: ", err)
	}

	// Turn off the "Lock on leave" feature. This disables HPS and gives us
	// traditional dimming behaviour (i.e. not quick dim).
	req.Enable = false
	if _, err := client.StartUIWithCustomScreenPrivacySetting(hctx.Ctx, req, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to change setting: ", err)
	}

	brightness, err := utils.GetBrightness(hctx.Ctx, dut.Conn())
	if err != nil {
		s.Fatal("Error failed to get brightness: ", err)
	}

	// Render hps-internal page for debugging before waiting for dim.
	if _, err := client.OpenHPSInternalsPage(hctx.Ctx, &empty.Empty{}); err != nil {
		s.Fatal("Error open hps-internals")
	}

	// It should *not* dim the screen within the quick dim delay period.
	// Poll instead of Sleep in order to fail fast in case screen dims before the delay.
	err = utils.PollForBrightnessChange(ctx, brightness, quickDimMetrics.DimDelay.AsDuration(), dut.Conn())
	if err != nil {
		// If brightness is not changed it's not an error in this case, as we'll check the brightness after polling anyway.
		testing.ContextLog(ctx, err.Error())
	}
	newBrightness, err := utils.GetBrightness(hctx.Ctx, dut.Conn())
	if err != nil {
		s.Fatal("Error when getting brightness: ", err)
	}
	if newBrightness != brightness {
		s.Fatal("Unexpected brightness change: was: ", brightness, ", new: ", newBrightness)
	}

	// It should *not* lock the screen within the quick screen off delay period.
	err = utils.PollForBrightnessChange(ctx, brightness, quickDimMetrics.ScreenOffDelay.AsDuration(), dut.Conn())
	if err != nil {
		// If brightness is not changed it's not an error in this case, as we'll check the brightness after polling anyway.
		testing.ContextLog(ctx, err.Error())
	}
	newBrightness, err = utils.GetBrightness(hctx.Ctx, dut.Conn())
	if err != nil {
		s.Fatal("Error when getting brightness: ", err)
	}
	if newBrightness != brightness {
		s.Fatal("Unexpected brightness change: was: ", brightness, ", new: ", newBrightness)
	}
}
