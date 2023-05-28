// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/services/cros/arc"
	arcpb "go.chromium.org/tast-tests/cros/services/cros/arc"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ADBOverUSB,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that arc(vm)-adbd job is up and running when adb-over-usb feature available",
		Contacts:     []string{"arc-core@google.com", "vraheja@google.com"},
		// ChromeOS > Software > ARC++ > Core > Integration
		BugComponent: "b:1131321",
		HardwareDeps: hwdep.D(
			// Available boards info, please refer to doc https://www.chromium.org/chromium-os/chrome-os-systems-supporting-adb-debugging-over-usb
			hwdep.Model("eve", "atlas", "nocturne", "soraka"),
		),
		SoftwareDeps: []string{"reboot", "chrome"},
		ServiceDeps:  []string{"tast.cros.arc.ADBOverUSBService"},
		// TODO(b/239203016): Stabilize the test.
		//Attr:         []string{"group:mainline", "informational"},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_p"},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
		Fixture: fixture.DevModeGBB,
		Timeout: 20 * time.Minute,
	})
}

func ADBOverUSB(ctx context.Context, s *testing.State) {
	d := s.DUT()
	// Connect to the gRPC server on the DUT
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	service := arc.NewADBOverUSBServiceClient(cl.Conn)
	enableUDCRequest := arcpb.EnableUDCRequest{
		Enable: true,
	}
	rsp, err := service.SetUDCEnabled(ctx, &enableUDCRequest)
	if err != nil {
		s.Fatal("Failed to enable USB Device Controller on the DUT: ", err)
	}

	if rsp.UDCValueUpdated {
		s.Log("Rebooting")
		if err := s.DUT().Reboot(ctx); err != nil {
			s.Fatal("Failed to reboot DUT: ", err)
		}

		// Reconnect to the gRPC server after rebooting DUT.
		cl, err = rpc.Dial(ctx, d, s.RPCHint())
		if err != nil {
			s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
		}
		defer cl.Close(ctx)

		service = arc.NewADBOverUSBServiceClient(cl.Conn)

		defer func() {
			disableUDCRequest := arcpb.EnableUDCRequest{
				Enable: false,
			}
			if _, err := service.SetUDCEnabled(ctx, &disableUDCRequest); err != nil {
				s.Fatal("Failed to disable USB Device Controller on the DUT: ", err)
			}
			s.Log("Rebooting")
			if err := s.DUT().Reboot(ctx); err != nil {
				s.Fatal("Failed to reboot DUT: ", err)
			}
		}()
	}

	s.Log("Checking arc(vm)-adbd job on DUT")
	if _, err = service.CheckADBDJobStatus(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to check arc(vm)-adbd job on DUT: ", err)
	}
}
