// Copyright 2022 The ChromiumOS Authors
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
	// strengthMargin is the allowable margin of error for signal strength percent.
	strengthMargin = 1.0
	// rsrpMargin is the allowable margin of error to use while waiting for RSRP to update to requested value in dBm.
	rsrpMargin = 3.0
)

// rxPower fetches the current signal power received at the dut in dBm.
type rxPower func(context.Context, cellular.RemoteCellularServiceClient) (float64, error)

// signalStrengthTest is a single test case in an attenuated signal strength test.
type signalStrengthTest struct {
	maxPower    float64
	minPower    float64
	stepSize    float64
	stepCount   int
	fetchPower  rxPower
	callboxOpts *manager.ConfigureCallboxRequestBody
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         AttenuatedSignalStrength,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Progressively lowers the downlink power on the callbox and verifies that the signal strength calculated by shill decreases by a proportional amount",
		Contacts:     []string{"chromeos-cellular-team@google.com", "jstanko@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_callbox", "cellular_cmw_callbox"},
		ServiceDeps:  []string{"tast.cros.cellular.RemoteCellularService"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "callboxManagedFixture",
		// FM101 only reports RSSI to MM (b/274882743), this test relies on RSRP values.
		HardwareDeps: hwdep.D(hwdep.SkipOnCellularModemType(cellularconst.ModemTypeFM101)),
		Timeout:      15 * time.Minute,
		Params: []testing.Param{
			{
				Name: "lte",
				Val: signalStrengthTest{
					maxPower: -88,
					minPower: -128,
					// decrease by ~20%, smaller step sizes may not be accurately resolvable if they're not at least 2*rsrpMargin
					stepSize:   -8,
					stepCount:  4,
					fetchPower: fetchLteRSRP,
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

func AttenuatedSignalStrength(ctx context.Context, s *testing.State) {
	tc := s.Param().(signalStrengthTest)
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
	pMeas, err := waitForPower(ctx, tc, tf.RemoteCellularClient, pReq)
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
		pMeas, err = waitForPower(ctx, tc, tf.RemoteCellularClient, pReq)
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
		s.Logf("Power: %f, strength: %d %%, offset: %f dBm", pMeas, strength, calibrationOffset)

		// if we're > 90 or at 0 then we may have been clipped
		if strength >= 90 {
			continue
		}
		if strength == 0 {
			break
		}

		if math.Abs(sDiffExpected-sDiff) > strengthMargin {
			s.Fatalf("Failed to change signal strength, expected strength: %f+/-%f%%, got: %f%%", sDiffExpected, strengthMargin, sDiff)
		}
	}
}

// fetchLteRSRP fetches the RSRP for the current LTE signal.
func fetchLteRSRP(ctx context.Context, client cellular.RemoteCellularServiceClient) (float64, error) {
	resp, err := client.QueryLTESignal(ctx, &empty.Empty{})
	if err != nil {
		return 0, err
	}
	return resp.Rsrp, nil
}

// waitForPower waits for the received power at the DUT to update within some margin of the requested value.
func waitForPower(ctx context.Context, test signalStrengthTest, client cellular.RemoteCellularServiceClient, pTarget float64) (float64, error) {
	var pMeas float64
	var err error
	if err = testing.Poll(ctx, func(ctx context.Context) error {
		if pMeas, err = test.fetchPower(ctx, client); err != nil {
			return errors.Wrap(err, "failed to get DUT Rx signal properties")
		}

		if math.Abs(pTarget-pMeas) > rsrpMargin {
			return errors.Errorf("waiting for DUT Rx power to reach %f, got %f", pTarget, pMeas)
		}

		return nil
	}, &testing.PollOptions{Interval: time.Second}); err != nil {
		return 0, errors.Wrap(err, "failed to wait for uplink power to reach the requested value")
	}

	return pMeas, nil
}
