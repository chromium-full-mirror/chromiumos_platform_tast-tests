// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"math"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/remote/cellular/callbox/manager"
	"go.chromium.org/tast-tests/cros/services/cros/cellular"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/cellularconst"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	// strengthErrorMargin is the allowable margin of error for signal strength percent.
	strengthErrorMargin = 1.0
	// rsrpErrorMargin is the allowable margin of error to use while waiting for RSRP to update to requested value in dBm.
	rsrpErrorMargin = 3.0
)

// rxSignalPower fetches the current signal power received at the dut in dBm.
type rxSignalPower func(context.Context, cellular.RemoteCellularServiceClient) (float64, error)

// signalBarTest is a single test case in an signal bar test.
type signalBarTest struct {
	maxPower    float64
	minPower    float64
	stepSize    float64
	stepCount   int
	fetchPower  rxSignalPower
	callboxOpts *manager.ConfigureCallboxRequestBody
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         AssertUISignalQuality,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Decrease downlink power on the callbox and verifies that the signal strength reflected on ui proportionally",
		Contacts:     []string{"chromeos-cellular-team@google.com", "srikanthkumar@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_callbox", "cellular_cmw_callbox", "cellular_e2e"},
		ServiceDeps:  []string{"tast.cros.cellular.RemoteCellularService"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "callboxManagedFixture",
		// FM101 only reports RSSI to MM (b/274882743), this test relies on RSRP values.
		HardwareDeps: hwdep.D(hwdep.SkipOnCellularModemType(cellularconst.ModemTypeFM101)),
		Timeout:      15 * time.Minute,
		Params: []testing.Param{
			{
				Name: "lte",
				Val: signalBarTest{
					maxPower: -88,
					minPower: -128,
					// decrease by ~20%, smaller step sizes may not be accurately resolvable if they're not at least 2*rsrpErrorMargin
					stepSize:   -8,
					stepCount:  3,
					fetchPower: fetchLteRSRPValue,
					callboxOpts: &manager.ConfigureCallboxRequestBody{
						Hardware:     manager.CallboxHardwareCMW,
						CellularType: manager.CellularTechnologyLTE,
						Parameters: []manager.CellConfiguration{
							manager.NewLteCellConfiguration(),
						},
					},
				},
			},
		},
	})
}

func AssertUISignalQuality(ctx context.Context, s *testing.State) {
	tc := s.Param().(signalBarTest)
	tf := s.FixtValue().(*manager.TestFixture)
	dutConn := s.DUT().Conn()

	if err := tf.ConnectToCallbox(ctx, dutConn, tc.callboxOpts); err != nil {
		s.Fatal("Failed to initialize cellular connection: ", err)
	}

	// get initial power set on the callbox
	// NOTE: callbox power is in RS EPRE while lte is reported by the modem in RSRP, the two should be nearly identical in this scenario
	rxResp, err := tf.CallboxManagerClient.FetchRxPower(ctx, &manager.FetchRxPowerRequestBody{})
	if err != nil {
		s.Fatal("Failed to fetch callbox downlink power: ", err)
	}
	pReq := rxResp.Power

	// wait for received power at the DUT to update
	pMeas, err := waitForSignalPower(ctx, tc, tf.RemoteCellularClient, pReq)
	if err != nil {
		s.Fatal("Failed to wait for requested power: ", err)
	}
	serviceResp, err := tf.RemoteCellularClient.QueryService(ctx, &empty.Empty{})
	if err != nil {
		s.Fatal("Failed to get cellular service properties: ", err)
	}
	strength := serviceResp.Strength
	calibrationOffset := pReq - pMeas

	s.Logf("Starting power: %f dBm, strength: %d %%, incrementing by %f dBm", pMeas, strength, tc.stepSize)
	for i := 0; i < tc.stepCount; i++ {
		pReq += tc.stepSize
		req := &manager.ConfigureRxPowerRequestBody{Power: manager.NewRxPower(pReq + calibrationOffset)}
		if err := tf.CallboxManagerClient.ConfigureRxPower(ctx, req); err != nil {
			s.Fatal("Failed to change callbox uplink power: ", err)
		}
		powerOld := pMeas
		pMeas, err = waitForSignalPower(ctx, tc, tf.RemoteCellularClient, pReq)
		if err != nil {
			s.Fatal("Failed to wait for requested power: ", err)
		}

		calibrationOffset = pReq + calibrationOffset - pMeas
		// calculate expected decrease in signal strength
		sDiffExpected := 100 * (powerOld - pMeas) / (tc.maxPower - tc.minPower)
		serviceResp, err := tf.RemoteCellularClient.QueryService(ctx, &empty.Empty{})
		if err != nil {
			s.Fatal("Failed to get cellular service properties: ", err)
		}
		sDiff := float64(strength - serviceResp.Strength)
		strength = serviceResp.Strength
		var expectedCount int32
		expectedCount = barCount(ctx, float64(strength))
		s.Logf("Power: %f, strength: %d %%, offset: %f dBm", pMeas, strength, calibrationOffset)
		resultedCount, err := getShillBasedBarCnt(ctx, tf.RemoteCellularClient)
		if err != nil {
			s.Log("Failed to get shill based cellular signal bar count: ", err)
		}
		s.Log("Getting signal bar count from UI")
		signalResp, err := tf.RemoteCellularClient.QuerySignalBars(ctx, &empty.Empty{})
		if err != nil {
			s.Fatal("Failed to get cellular signal bar count: ", err)
		}
		uiBarCnt := int32(signalResp.Count)
		s.Logf("Signal bar count want: %d, got shill based count: %d, ui based count: %d", expectedCount, resultedCount, uiBarCnt)
		if uiBarCnt <= 0 {
			s.Fatalf("Cellular network not having any signal: %d", uiBarCnt)
		}
		if expectedCount != uiBarCnt || expectedCount != resultedCount {
			s.Fatal("Failed to match signal bar on ui")
		}

		if math.Abs(sDiffExpected-sDiff) > strengthErrorMargin {
			s.Fatalf("Failed to change signal strength, want: %f+/-%f%%, got: %f%%", sDiffExpected, strengthErrorMargin, sDiff)
		}
	}
}

// getShillBasedBarCnt fetches the RSRP for the current LTE signal strngth and maps expected signal bars.
func getShillBasedBarCnt(ctx context.Context, client cellular.RemoteCellularServiceClient) (int32, error) {
	resp, err := client.QueryLTESignal(ctx, &empty.Empty{})
	if err != nil {
		return 0, err
	}
	var min, max, clampedSignalQuality, signalPercent float64 = -128, -88, 0, 0
	clampedSignalQuality = math.Min(math.Max(resp.Rsrp, min), max)
	signalPercent = (clampedSignalQuality - min) * 100 / (max - min)
	testing.ContextLogf(ctx, "clampedSignalQuality: %f,signalPercent: %f , max: %f, min: %f, Rsrp: %f", clampedSignalQuality, signalPercent, max, min, resp.Rsrp)
	return barCount(ctx, signalPercent), nil
}

// barCount maps shill signal strength with number of signal bars on UI.
func barCount(ctx context.Context, strength float64) int32 {
	// Chrome OS UI uses signal quality values set by this method to draw
	// network icons. UI code maps |quality| to number of bars as follows:
	// [1-25] 1 bar, [26-50] 2 bars, [51-75] 3 bars and [76-100] 4 bars.
	// -128->-88 rsrp scales to UI quality of 0->100, used for 4G
	// -115->-89 rscp scales to UI quality of 0->100, used for 3G
	// -105->-83 rssi scales to UI quality of 0->100, used for other tech
	return (int32(strength) + 24) / 25
}

// fetchLteRSRPValue fetches the RSRP for the current LTE signal.
func fetchLteRSRPValue(ctx context.Context, client cellular.RemoteCellularServiceClient) (float64, error) {
	resp, err := client.QueryLTESignal(ctx, &empty.Empty{})
	if err != nil {
		return 0, err
	}
	return resp.Rsrp, nil
}

// waitForSignalPower waits for the received power at the DUT to update within some margin of the requested value.
func waitForSignalPower(ctx context.Context, test signalBarTest, client cellular.RemoteCellularServiceClient, pTarget float64) (float64, error) {
	var pMeas float64
	var err error
	if err = testing.Poll(ctx, func(ctx context.Context) error {
		if pMeas, err = test.fetchPower(ctx, client); err != nil {
			return errors.Wrap(err, "failed to get DUT Rx signal properties")
		}

		if math.Abs(pTarget-pMeas) > rsrpErrorMargin {
			return errors.Errorf("waiting for DUT Rx power to reach %f, got %f", pTarget, pMeas)
		}

		return nil
	}, &testing.PollOptions{Interval: time.Second}); err != nil {
		return 0, errors.Wrap(err, "failed to wait for uplink power to reach the requested value")
	}

	return pMeas, nil
}
