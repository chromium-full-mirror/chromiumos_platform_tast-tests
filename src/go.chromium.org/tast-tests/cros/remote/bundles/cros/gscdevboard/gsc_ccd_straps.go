// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/testing"
)

type gSCCCDStrapsParam struct {
	strap         ti50.GpioStrap
	expectedState ti50.UsbDeviceLinkState
}

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCCCDStraps,
		Desc:    "Test that the GSC identifies all valid CCD strap configurations",
		Timeout: 10 * time.Second,
		Contacts: []string{
			"gsc-sheriff@google.com", // CrOS GSC Developers
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_dt_ab", "gsc_dt_shield", "gsc_ot_shield", "gsc_image_ti50", "gsc_nightly"},
		Fixture:      fixture.GSCOpenCCD,
		Params: []testing.Param{{
			Name: "suzyq",
			Val: gSCCCDStrapsParam{
				strap:         ti50.CcdSuzyQ,
				expectedState: ti50.SuzyQConnected,
			},
		}, {
			Name: "suzyq_flipped",
			Val: gSCCCDStrapsParam{
				strap:         ti50.CcdSuzyQFlipped,
				expectedState: ti50.UsbDisconnected,
			},
		}, {
			Name: "servo",
			Val: gSCCCDStrapsParam{
				strap:         ti50.CcdServo,
				expectedState: ti50.ServoConnected,
			},
		}, {
			Name: "servo_flipped",
			Val: gSCCCDStrapsParam{
				strap:         ti50.CcdServoFlipped,
				expectedState: ti50.ServoFlippedConnected,
			},
		}, {
			Name: "servo_sink1",
			Val: gSCCCDStrapsParam{
				strap:         ti50.CcdServoSnk1,
				expectedState: ti50.ServoSink1Connected,
			},
		}, {
			Name: "servo_sink2",
			Val: gSCCCDStrapsParam{
				strap:         ti50.CcdServoSnk2,
				expectedState: ti50.ServoSink2Connected,
			},
		}, {
			Name: "servo_sink3",
			Val: gSCCCDStrapsParam{
				strap:         ti50.CcdServoSnk3,
				expectedState: ti50.ServoSink3Connected,
			},
		}},
	})
}

func GSCCCDStraps(ctx context.Context, s *testing.State) {
	userParams := s.Param().(gSCCCDStrapsParam)
	b := utils.NewDevboardHelper(s)
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}

	s.Log("Restarting GSC")
	b.GpioApplyStrap(ctx, ti50.CcdDisconnected)
	b.Reset(ctx)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	// Make sure that we report that we're disconnected first
	usbAdcInfo, err := i.USBADCInfo(ctx)
	th.MustSucceed(err, "Error communicating with GSC")
	if usbAdcInfo.State != ti50.UsbDisconnected {
		s.Error("Expected GSC to report CCD disconnect, but was: ", usbAdcInfo.State)
	}

	// Set the strap to test
	b.GpioApplyStrap(ctx, userParams.strap)
	th.MustSucceed(testing.Sleep(ctx, 2*time.Second), "Context expired while waiting for GSC to process the strap change") // GoBigSleepLint: Wait for GSC to process the strap change

	// Check the we report the new USB ADC link state
	usbAdcInfo, err = i.USBADCInfo(ctx)
	th.MustSucceed(err, "Error communicating with GSC")
	if usbAdcInfo.State != userParams.expectedState {
		s.Errorf("Expected GSC to report %v state with no reboot, but was %v", userParams.expectedState, usbAdcInfo.State)
	}

	// Reboot the device again without changing the strap
	s.Log("Restarting GSC without changing CCD strap")
	b.Reset(ctx)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	// Check that we read the correct strapping
	usbAdcInfo, err = i.USBADCInfo(ctx)
	th.MustSucceed(err, "Error communicating with GSC")
	if usbAdcInfo.State != userParams.expectedState {
		s.Errorf("Expected GSC to report %v state after reboot, but was %v", userParams.expectedState, usbAdcInfo.State)
	}
}
