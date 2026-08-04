// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	bts "go.chromium.org/tast-tests/cros/services/cros/bluetooth"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           OobeHidBluetoothAdapterState,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Checks bluetooth adapter states updates correctly in OOBE",
		Contacts: []string{
			"alfredyu@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent: "b:1578688", // ChromeOS > External > Cienet > Manual Test Automation > Test stabilization
		// TODO(b/542029943): Re-enable once the test is stable.
		// Attr:         []string{"group:mainline", "informational"},
		TestBedDeps:  []string{tbdep.BluetoothStateNormal},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps: []string{
			"tast.cros.bluetooth.OobeHidBluetoothService",
			"tast.cros.ui.ChromeUIService",
		},
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		Fixture:      "turnOffServoKeyboard",
		HardwareDeps: hwdep.D(hwdep.FormFactor(hwdep.Chromebox, hwdep.Chromebit)),
	})
}

// OobeHidBluetoothAdapterState tests that Bluetooth adapter is enabled in OOBE
// hid detection screen.
func OobeHidBluetoothAdapterState(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	util := newOobeHidBluetoothAdapterStateTestUtil(s.DUT(), s.RPCHint())
	defer util.cleanup(cleanupCtx)

	if err := util.init(ctx); err != nil {
		s.Fatal("Failed to initialize test utilities: ", err)
	}

	newChromeRequest := &bts.NewChromeRequest{
		SigninProfileTestExtension: s.RequiredVar("ui.signinProfileTestExtensionManifestKey"),
	}

	// Isolate the step to leverage `defer` pattern.
	func() {
		if _, err := util.NewChrome(ctx, newChromeRequest); err != nil {
			s.Fatal("Failed to create new chrome instance: ", err)
		}
		defer util.CloseChrome(cleanupCtx, &emptypb.Empty{})
		defer util.dumpUITreeWithScreenshotToFile(cleanupCtx, s.HasError, "ui_dump")

		if _, err := util.ProgressToWelcomeScreen(ctx, &emptypb.Empty{}); err != nil {
			s.Fatal("Failed to progress to welcome screen: ", err)
		}

		if _, err := util.DisableBluetoothFromQuickSettings(ctx, &emptypb.Empty{}); err != nil {
			s.Fatal("Failed to disabled bluetooth adapter from quick settings: ", err)
		}
	}()

	if err := util.rebootDUT(ctx); err != nil {
		s.Fatal("Failed to reboot DUT: ", err)
	}

	if _, err := util.NewChrome(ctx, newChromeRequest); err != nil {
		s.Fatal("Failed to create new chrome instance: ", err)
	}
	defer util.CloseChrome(cleanupCtx, &emptypb.Empty{})
	defer util.dumpUITreeWithScreenshotToFile(cleanupCtx, s.HasError, "ui_dump")

	if _, err := util.VerifyBluetoothIsEnabled(ctx, &emptypb.Empty{}); err != nil {
		s.Fatal("Failed to verify bluetooth is enabled: ", err)
	}
}

type oobeHidBluetoothAdapterStateTestUtil struct {
	bts.OobeHidBluetoothServiceClient
	rpcClient *rpc.Client

	dut     *dut.DUT
	rpcHint *testing.RPCHint
}

func newOobeHidBluetoothAdapterStateTestUtil(dut *dut.DUT, rpcHint *testing.RPCHint) *oobeHidBluetoothAdapterStateTestUtil {
	return &oobeHidBluetoothAdapterStateTestUtil{dut: dut, rpcHint: rpcHint}
}

func (util *oobeHidBluetoothAdapterStateTestUtil) cleanup(ctx context.Context) error {
	if util.OobeHidBluetoothServiceClient != nil {
		util.OobeHidBluetoothServiceClient = nil
	}

	if util.rpcClient != nil {
		if err := util.rpcClient.Close(ctx); err != nil {
			return err
		}
		util.rpcClient = nil
	}

	return nil
}

func (util *oobeHidBluetoothAdapterStateTestUtil) init(ctx context.Context) error {
	// Skip if resources have been initialized.
	if util.rpcClient != nil && util.OobeHidBluetoothServiceClient != nil {
		return nil
	}

	if util.dut == nil || util.rpcHint == nil {
		return errors.New("invalid DUT connection or RPC hint")
	}

	rpcClient, err := rpc.Dial(ctx, util.dut, util.rpcHint)
	if err != nil {
		return errors.Wrap(err, "failed to connect to the RPC service on the DUT")
	}

	util.rpcClient = rpcClient
	util.OobeHidBluetoothServiceClient = bts.NewOobeHidBluetoothServiceClient(util.rpcClient.Conn)
	return nil
}

func (util *oobeHidBluetoothAdapterStateTestUtil) rebootDUT(ctx context.Context) error {
	if err := util.cleanup(ctx); err != nil {
		// Only logs the error as it won't interfere the reboot and resources will be reinitialized afterward.
		testing.ContextLog(ctx, "Failed to cleanup test util: ", err)
	}

	if err := util.dut.Reboot(ctx); err != nil {
		return errors.Wrap(err, "failed to reboot DUT")
	}

	// Reconnect to the gRPC server after rebooting DUT.
	return util.init(ctx)
}

func (util *oobeHidBluetoothAdapterStateTestUtil) dumpUITreeWithScreenshotToFile(ctx context.Context, hasError func() bool, filePrefix string) error {
	if !hasError() {
		return nil
	}

	svc := ui.NewChromeUIServiceClient(util.rpcClient.Conn)
	if _, err := svc.DumpUITreeWithScreenshotToFile(ctx, &ui.DumpUITreeWithScreenshotToFileRequest{FilePrefix: filePrefix}); err != nil {
		return err
	}
	return nil
}
