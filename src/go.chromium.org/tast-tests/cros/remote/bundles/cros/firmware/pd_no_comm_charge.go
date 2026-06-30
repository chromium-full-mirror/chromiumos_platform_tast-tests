// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/services/cros/power"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: PDNoCommCharge,
		Desc: "Test that emulates a 5V charger without PD",
		Contacts: []string{
			"chromeos-faft@google.com", // Owning team list
			"jasonyuan@google.com",     // Test author
		},
		BugComponent: "b:792402", // ChromeOS > Platform > baseOS > Firmware > FAFT
		Data:         []string{firmware.ConfigFile},
		Vars:         []string{"servo"},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps: []string{
			"tast.cros.power.BatteryService",
		},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.Battery()),
		Timeout:      60 * time.Minute,
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		// TODO(b/156552219): Add "group:firmware_pd" tag once the test is stable.
		Attr: []string{"group:firmware", "firmware_pd_unstable"},
		Params: []testing.Param{{
			Name: "normal",
			Val:  firmware.PDTestParams{},
		}, {
			Name: "flipcc",
			Val: firmware.PDTestParams{
				CC: firmware.CCPolarityFlipped,
			},
		}, {
			Name: "dtsoff",
			Val: firmware.PDTestParams{
				DTS: firmware.DTSModeOff,
			},
		}, {
			Name: "flipcc_dtsoff",
			Val: firmware.PDTestParams{
				CC:  firmware.CCPolarityFlipped,
				DTS: firmware.DTSModeOff,
			},
		}, {
			Name: "shutdown",
			Val: firmware.PDTestParams{
				Shutdown: true,
				DTS:      firmware.DTSModeOff,
			},
		}},
	})
}

const (
	// usbcPollTimeout is the timeout for a usbc connection
	usbcPollTimeout time.Duration = 10 * time.Second
	// usbcPollInterval is the time before testing for a usbc connection
	usbcPollInterval time.Duration = 1 * time.Second
)

func PDNoCommCharge(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	if err := h.RequireRPCClient(ctx); err != nil {
		s.Fatal("Failed to connect to RPC: ", err)
	}
	client := power.NewBatteryServiceClient(h.RPCClient.Conn)
	if _, err := client.New(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to start BatteryServiceClient: ", err)
	}
	defer client.Close(ctx, &empty.Empty{})
	if _, err := client.StopChargeLimit(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to stop charge limit: ", err)
	}

	// If battery is full, discharge it some before starting the test
	if err := firmware.DischargeBattery(ctx, h, 96.0); err != nil {
		s.Fatal("Failed discharge battery: ", err)
	}

	testParams := s.Param().(firmware.PDTestParams)

	if err := firmware.SetupPDTester(ctx, h, testParams, s.OutDir()); err != nil {
		s.Fatal("Failed to configure Servo for PD testing: ", err)
	}

	testing.ContextLog(ctx, "swapping to 5V usbc charging")
	dtsBool := true
	if testParams.DTS == firmware.DTSModeOff {
		dtsBool = false
	}
	defer func() {
		// restore normal servo charging functionality
		testing.ContextLog(ctx, "restoring PD communication")
		h.Servo.ServoCcOff(ctx)
		if err := h.Servo.SetPDRole(ctx, servo.PDRoleSrc); err != nil {
			s.Fatal("Failed to restore PD comm: ", err)
		}
	}()

	configs := []servo.USBCCurrentAdvertisement{
		servo.USBCusb,
		servo.USBC1A5,
		servo.USBC3A0,
	}

	// pass if current draw is within 80% ~ 110% of advertised current
	expectedCurrent := [][]int{
		{400, 550},
		{1200, 1650},
		{2400, 3300},
	}

	for idx := range configs {
		testing.ContextLogf(ctx, "testing %s connnection", string(configs[idx]))
		if err := h.Servo.ServoCCNoPD(ctx, dtsBool, configs[idx]); err != nil {
			s.Fatal("Could not initialize charging without PD: ", err)
		}

		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if INAInfo, err := h.Servo.ServoGetINA(ctx, 1); err == nil {
				if 4500 > INAInfo.BusMV || INAInfo.BusMV > 5500 {
					return errors.Wrapf(err, "expected connection to be 5V, it is instead %dmV", INAInfo.BusMV)
				}
				if expectedCurrent[idx][0] > INAInfo.CurrentMA || INAInfo.CurrentMA > expectedCurrent[idx][1] {
					return errors.Wrapf(err, "expected current draw %dmA, it is instead %dmA", expectedCurrent[idx][1], INAInfo.CurrentMA)
				}
			} else {
				return errors.Wrap(err, "failed to get charging connection")
			}

			return nil
		}, &testing.PollOptions{Timeout: usbcPollTimeout, Interval: usbcPollInterval}); err != nil {
			s.Fatal("Failed to acquire correct connection: ", err)
		}
	}

	if testParams.Shutdown {
		if err := h.Servo.SetPowerState(ctx, servo.PowerStateOn); err != nil {
			testing.ContextLog(ctx, "Failed to power on DUT: ", err)
		}
		if err := h.WaitConnect(ctx); err != nil {
			s.Fatal("Failed to boot after test: ", err)
		}
	}
}
