// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/protobuf/types/known/emptypb"

	bts "go.chromium.org/tast-tests/cros/services/cros/bluetooth"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           OobeHidBluetoothAdapterState,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Checks bluetooth adapter states updates correctly in OOBE",
		Contacts: []string{
			"cros-connectivity@google.com",
			"tjohnsonkanu@google.com",
		},
		BugComponent: "b:1131776", // ChromeOS > Software > System Services > Connectivity > Bluetooth
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps: []string{
			"tast.cros.bluetooth.OobeHidBluetoothService",
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
	ctx, cancel := ctxutil.Shorten(cleanupCtx, time.Second*10)
	defer cancel()

	var rpcClient *rpc.Client
	var service bts.OobeHidBluetoothServiceClient
	var err error

	createServiceAndStartChrome := func() {
		rpcClient, err = rpc.Dial(ctx, s.DUT(), s.RPCHint())
		if err != nil {
			s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
		}

		service = bts.NewOobeHidBluetoothServiceClient(rpcClient.Conn)
		if _, err := service.NewChrome(ctx, &bts.NewChromeRequest{
			SigninProfileTestExtension: s.RequiredVar("ui.signinProfileTestExtensionManifestKey"),
		}); err != nil {
			s.Fatal("Failed to create new chrome instance: ", err)
		}
	}

	createServiceAndStartChrome()
	func() {
		defer rpcClient.Close(cleanupCtx)

		if _, err := service.ProgressToWelcomeScreen(ctx, &emptypb.Empty{}); err != nil {
			s.Fatal("Failed to progress to welcome screen: ", err)
		}

		if _, err := service.DisableBluetoothFromQuickSettings(ctx, &emptypb.Empty{}); err != nil {
			s.Fatal("Failed to disabled bluetooth adapter from quick settings: ", err)
		}
	}()

	if err := s.DUT().Reboot(ctx); err != nil {
		s.Fatal("Failed to reboot DUT: ", err)
	}

	// Reconnect to the gRPC server after rebooting DUT.
	createServiceAndStartChrome()
	defer rpcClient.Close(cleanupCtx)
	defer service.CloseChrome(cleanupCtx, &empty.Empty{})

	if _, err := service.VerifyBluetoothIsEnabled(ctx, &emptypb.Empty{}); err != nil {
		s.Fatal("Failed to verify bluetooth is enabled: ", err)
	}
}
