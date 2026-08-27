// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/utils"
	"go.chromium.org/tast/core/testing"
)

type ccRange struct {
	// lowerBoundVolts is the lower end of the voltage range guaranteed to be recognized.
	lowerBoundVolts float32
	// upperBoundVolts is the upper end of the voltage range guaranteed to be recognized.
	upperBoundVolts float32
	// margin is the band outside either end of the range defined above, for which we know
	// that no other types of cables should appear.  It is acceptable if the GSC interprets a
	// voltage within the margin as belonging to the detection range, but it is not acceptable
	// for the GSC interpret any voltage outside of the margin as a CCD cable.
	margin float32
}

type gSCCCDStrapsParam struct {
	cc1           ccRange
	cc2           ccRange
	expectedState ti50.UsbDeviceLinkState
}

// Voltage ranges lifted from the diagram in section "Potential Improvement: Change the Resistors"
// in the below document:
// https://docs.google.com/document/d/1KRKCMJG85X7cgCFhVENLpDybfO4jm9wK5Nbd4RVyC90/edit?tab=t.0#bookmark=id.77qu8h5t9siz

var suzyqL = ccRange{
	lowerBoundVolts: 0.400,
	upperBoundVolts: 0.600,
	margin:          0.030,
}

var suzyqH = ccRange{
	lowerBoundVolts: 0.750,
	upperBoundVolts: 1.120,
	margin:          0.030,
}

var servoL = ccRange{
	lowerBoundVolts: 0.870,
	upperBoundVolts: 1.080,
	// TODO(b/370810285) This margin could be too large "below", and go into the "cyan" range
	// that we may want to guarantee is not detected as CCD.  It should be reduced to at most
	// 140mV and GSC code adapted accordingly.
	margin: 0.160,
}

var servoH = ccRange{
	lowerBoundVolts: 1.530,
	upperBoundVolts: 1.820,
	margin:          0.200,
}

var servoSnk1 = ccRange{
	lowerBoundVolts: 0.320,
	upperBoundVolts: 0.600,
	margin:          0.030,
}

var servoSnk2 = ccRange{
	lowerBoundVolts: 0.840,
	upperBoundVolts: 1.130,
	margin:          0.150,
}

var servoSnk3 = ccRange{
	lowerBoundVolts: 1.530,
	upperBoundVolts: 2.000,
	margin:          0.200,
}

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCCCDStraps,
		Desc:    "Test that the GSC identifies all valid CCD strap configurations",
		Timeout: 1 * time.Minute,
		Contacts: []string{
			"cros-hwsec@google.com", // CrOS GSC Developers
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_dt_ab", "gsc_dt_shield", "gsc_ot_shield", "gsc_image_ti50", "gsc_nightly"},
		Fixture:      fixture.GSCOpenCCD,
		Params: []testing.Param{{
			Name: "suzyq",
			Val: gSCCCDStrapsParam{
				cc1:           suzyqH,
				cc2:           suzyqL,
				expectedState: ti50.SuzyQConnected,
			},
		}, {
			Name: "servo_suzyq_flipped",
			Val: gSCCCDStrapsParam{
				cc1:           suzyqL,
				cc2:           suzyqH,
				expectedState: ti50.Servo1A5FlippedConnected,
			},
		}, {
			Name: "servo",
			Val: gSCCCDStrapsParam{
				cc1:           servoH,
				cc2:           servoL,
				expectedState: ti50.ServoConnected,
			},
		}, {
			Name: "servo_flipped",
			Val: gSCCCDStrapsParam{
				cc1:           servoL,
				cc2:           servoH,
				expectedState: ti50.ServoFlippedConnected,
			},
		}, {
			Name: "servo_sink1",
			Val: gSCCCDStrapsParam{
				cc1:           servoSnk1,
				cc2:           servoSnk1,
				expectedState: ti50.ServoSink1Connected,
			},
		}, {
			Name: "servo_sink2",
			Val: gSCCCDStrapsParam{
				cc1:           servoSnk2,
				cc2:           servoSnk2,
				expectedState: ti50.ServoSink2Connected,
			},
		}, {
			Name: "servo_sink3",
			Val: gSCCCDStrapsParam{
				cc1:           servoSnk3,
				cc2:           servoSnk3,
				expectedState: ti50.ServoSink3Connected,
			},
		}},
	})
}

var (
	reAdcMessage *regexp.Regexp = regexp.MustCompile(`ADC: (dis)?connected(.*)`)
)

func eventsMustBe(events utils.GpioEvents, expected []utils.GpioEdge, caseStr string, s *testing.State) {
	if len(events.Sorted) != len(expected) {
		s.Log("Got events: ", events)
		s.Errorf("Expected %s at %s", expected, caseStr)
		return
	}
	for i := 0; i < len(expected); i++ {
		if events.Sorted[i].Edge != expected[i] {
			s.Log("Got events: ", events)
			s.Errorf("Expected %s at %s", expected, caseStr)
			return
		}
	}
}

func GSCCCDStraps(ctx context.Context, s *testing.State) {
	userParams := s.Param().(gSCCCDStrapsParam)
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	// Run test with voltages "in the middle" of the allowed ranges.
	testStrapCorner(ctx, userParams.expectedState,
		(userParams.cc1.lowerBoundVolts+userParams.cc1.upperBoundVolts)/2,
		(userParams.cc2.lowerBoundVolts+userParams.cc2.upperBoundVolts)/2,
		s.TestName()+" center",
		b, i, s)

	// corner-case tests below are disabled, as they fail on Dauntless and on the OpenTitan in
	// Andrew's Satlab.
	//testInsideCorners(ctx, userParams, b, i, s)
	//testOutsideEdges(ctx, userParams, b, i, s)
}

func testStrapCorner(ctx context.Context, expectedState ti50.UsbDeviceLinkState, cc1Volts, cc2Volts float32, caseStr string, b utils.DevboardHelper, i *ti50.CrOSImage, s *testing.State) {
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}

	s.Log("Restarting GSC")
	b.GpioApplyStrap(ctx, ti50.CcdDisconnected, ti50.StrapReset)
	gpioMonitor := b.GpioMonitorStart(ctx, ti50.GpioTi50CcdModeL)
	defer b.GpioMonitorFinish(ctx, gpioMonitor)
	b.Reset(ctx)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	if b.TestbedType != ti50.GscH1Shield {
		// The Cr50 codebase does not implement ADC reading through the console, skip and
		// rely instead on GPIO monitoring of CCD_MODE_L to verify detection.

		// Make sure that we report that we're disconnected first
		usbAdcInfo, err := i.USBADCInfo(ctx)
		th.MustSucceed(err, "Error communicating with GSC")
		if usbAdcInfo.State != ti50.UsbDisconnected {
			s.Error("Expected GSC to report CCD disconnected, but was: ", usbAdcInfo.State)
		}
	}

	// Set the strap to test
	applyCCVoltages(ctx, cc1Volts, cc2Volts, b)
	th.MustSucceed(testing.Sleep(ctx, 2*time.Second), "Context expired while waiting for GSC to process the strap change") // GoBigSleepLint: Wait for GSC to process the strap change

	if b.TestbedType != ti50.GscH1Shield {
		// The Cr50 codebase does not implement ADC reading through the console, skip and
		// rely instead on GPIO monitoring of CCD_MODE_L to verify detection.

		// Check the we report the new USB ADC link state
		usbAdcInfo, err := i.USBADCInfo(ctx)
		th.MustSucceed(err, "Error communicating with GSC")
		if usbAdcInfo.State != expectedState {
			s.Errorf("Expected GSC to report %v state with no reboot, but was %v at %s", expectedState, usbAdcInfo.State, caseStr)
		}
	}
	if expectedState != ti50.UsbDisconnected {
		eventsMustBe(b.GpioMonitorRead(ctx, gpioMonitor), []utils.GpioEdge{utils.GpioEdgeFalling}, caseStr, s)
	} else {
		eventsMustBe(b.GpioMonitorRead(ctx, gpioMonitor), []utils.GpioEdge{}, caseStr, s)
	}

	// Reboot the device again without changing the strap
	s.Log("Restarting GSC without changing CCD strap")
	b.GpioApplyStrap(ctx, ti50.StrapReset)
	b.GpioMonitorRead(ctx, gpioMonitor) // Discard "artificial" rising edge upon reset.
	b.Reset(ctx)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	if b.TestbedType != ti50.GscH1Shield {
		// The Cr50 codebase does not implement ADC reading through the console, skip and
		// rely instead on GPIO monitoring of CCD_MODE_L to verify detection.

		// Check that we read the correct strapping
		usbAdcInfo, err := i.USBADCInfo(ctx)
		th.MustSucceed(err, "Error communicating with GSC")
		if usbAdcInfo.State != expectedState {
			s.Errorf("Expected GSC to report %v state after reboot, but was %v at %s", expectedState, usbAdcInfo.State, caseStr)
		}
	}
	if expectedState != ti50.UsbDisconnected {
		eventsMustBe(b.GpioMonitorRead(ctx, gpioMonitor), []utils.GpioEdge{utils.GpioEdgeFalling}, caseStr, s)
	} else {
		eventsMustBe(b.GpioMonitorRead(ctx, gpioMonitor), []utils.GpioEdge{}, caseStr, s)
	}

	// Simulate voltage momentarily venturing outside detection range.  Should neither cause
	// disconnection nor console output.
	var cc1VoltsOutside, cc2VoltsOutside float32
	if cc1Volts < 0.2 {
		cc1VoltsOutside = 1.0
	} else {
		cc1VoltsOutside = 0.0
	}
	if cc2Volts < 0.2 {
		cc2VoltsOutside = 1.0
	} else {
		cc2VoltsOutside = 0.0
	}
	b.GpioDACBang(ctx, "1kHz", fmt.Sprintf("%f %f 5ms %f %f", cc1VoltsOutside, cc2VoltsOutside, cc1Volts, cc2Volts), ti50.GpioTi50CC1, ti50.GpioTi50CC2)
	if match, err := i.WaitUntilMatch(ctx, reAdcMessage, 2*time.Second); err == nil {
		s.Errorf("ADC output during brief excursion: %s", match[0])
	}
	eventsMustBe(b.GpioMonitorRead(ctx, gpioMonitor), []utils.GpioEdge{}, caseStr+" brief excursion", s)

	// Simulate disconnection of CCD cable.
	b.GpioApplyStrap(ctx, ti50.CcdDisconnected)
	th.MustSucceed(testing.Sleep(ctx, 2*time.Second), "Context expired while waiting for GSC to process the strap change") // GoBigSleepLint: Wait for GSC to process the strap change

	if b.TestbedType != ti50.GscH1Shield {
		// The Cr50 codebase does not implement ADC reading through the console, skip and
		// rely instead on GPIO monitoring of CCD_MODE_L to verify detection.

		usbAdcInfo, err := i.USBADCInfo(ctx)
		th.MustSucceed(err, "Error communicating with GSC")
		if usbAdcInfo.State != ti50.UsbDisconnected {
			s.Error("Expected GSC to report CCD disconnected, but was: ", usbAdcInfo.State)
		}
	}
	if expectedState != ti50.UsbDisconnected {
		eventsMustBe(b.GpioMonitorRead(ctx, gpioMonitor), []utils.GpioEdge{utils.GpioEdgeRising}, caseStr+" disconnection", s)
	} else {
		eventsMustBe(b.GpioMonitorRead(ctx, gpioMonitor), []utils.GpioEdge{}, caseStr+" disconnection", s)
	}
}

func applyCCVoltages(ctx context.Context, cc1Volts, cc2Volts float32, b utils.DevboardHelper) {
	b.GpioAnalogSet(ctx, ti50.GpioTi50CC1, cc1Volts)
	b.GpioAnalogSet(ctx, ti50.GpioTi50CC2, cc2Volts)
}
