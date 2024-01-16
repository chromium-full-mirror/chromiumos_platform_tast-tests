// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package uwb is the set of remote tests that test UWB capabilities and functionality in ChromeOS
package uwb

import (
	"context"
	"time"

	"math/rand"

	"go.chromium.org/tast-tests/cros/common/uwb"
	ur "go.chromium.org/tast-tests/cros/remote/uwb"
	us "go.chromium.org/tast-tests/cros/services/cros/uwb"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrosToCrosRanging,
		Desc:         "Basic UWB Ranging Test between 2 CrOS devices",
		LacrosStatus: testing.LacrosVariantUnneeded,
		Contacts: []string{
			"chromeos-uwb-team@google.com",
		},
		Attr:         []string{"group:uwb", "uwb_cros_peers_1", "uwb_unstable"},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.uwb.UwbService"},
		Timeout:      5 * time.Minute,
		BugComponent: "b:1135853",
		Fixture:      "uwbMixedPeerRemote",
		Params: []testing.Param{{
			Name: "aoa",
			Val:  true,
		}, {
			Name: "no_aoa",
			Val:  false,
		},
		},
	})
}

func CrosToCrosRanging(ctx context.Context, s *testing.State) {
	//Check if AoA is tested, set aoa to true
	aoa := s.Param().(bool)

	uwbFixtValue := s.FixtValue().(*ur.FixtData)

	//Verify that devices set up in fixture match required minimum number of devices for this test (2 ChromeOS devices including main DUT)
	if len(uwbFixtValue.CrosClients) < 2 {
		s.Fatal("Number of CrosClients in FixtureValue is less than 2, indicating that an insufficient number of companion DUTs have been set up in the fixture")
	}

	// Retrieve actual Lab DUT unique names from fixtValue
	mainHostName := uwbFixtValue.MainHost
	cros1Name := uwbFixtValue.CrosHosts[0]

	// Get d-bus clients from fixture
	mainDutClient := *uwbFixtValue.CrosClients[0]
	cros1Client := *uwbFixtValue.CrosClients[1]

	// Set up fira ranging params and init requests
	controllerParams := uwb.FiraDefaultControllerSet()
	controleeParams := uwb.FiraDefaultControleeSet()

	// Generate random Session ID to avoid concurrent ranging sessions from different testbeds in the same lab conflicting with one another
	sessionID := uint32(rand.Intn(10000000) + 1)

	controllerReq := uwb.InitSessionRequest{
		SessionId:   sessionID,
		SessionType: uwb.SessionType_FIRA_RANGING_SESSION,
		Params:      controllerParams,
	}

	controleeReq := uwb.InitSessionRequest{
		SessionId:   sessionID,
		SessionType: uwb.SessionType_FIRA_RANGING_SESSION,
		Params:      controleeParams,
	}

	// Set AoA FiRA parameter
	if aoa {
		controllerReq.Params.AoaResultRequest = uwb.AoaResultRequest_REQ_AOA_RESULTS
		controleeReq.Params.AoaResultRequest = uwb.AoaResultRequest_REQ_AOA_RESULTS
	}

	// Init Main DUT as controller & initiator
	err := uwb.CallAndCheckOK(mainDutClient.InitSession(ctx, &controllerReq))
	if err != nil {
		s.Fatal("Init session dbus call failed for main dut: ", err)
	}

	// Init CrOS device 1 as controlee & responder
	err = uwb.CallAndCheckOK(cros1Client.InitSession(ctx, &controleeReq))
	if err != nil {
		s.Fatal("Init session dbus call failed for cros device 1: ", err)
	}

	startReq := uwb.StartRangingRequest{
		SessionId: sessionID,
	}

	// Start Ranging session on Main Dut
	err = uwb.CallAndCheckOK(mainDutClient.StartRanging(ctx, &startReq))
	if err != nil {
		s.Fatal("Start session dbus call failed for main dut: ", err)
	}

	// Start Ranging session on CrOS 1
	err = uwb.CallAndCheckOK(cros1Client.StartRanging(ctx, &startReq))
	if err != nil {
		s.Fatal("Start session dbus call failed for cros 1 device: ", err)
	}

	// GoBigSleepLint: uwb ranging takes a few seconds to begin, so we give it some time before collecting
	//the reports to avoid collecting reports where either device have yet to start
	testing.Sleep(ctx, 5*time.Second)

	// Get the expected distance and angle bounds for the 2 respective lab devices
	bounds, err := uwb.GetBoundsFromNames(mainHostName, cros1Name, uwb.DistanceAcceptableErrorMargin, uwb.AngleAcceptableErrorMargin, uwb.AngleAcceptableMinFOM)
	if err != nil {
		s.Fatal("Unable to create expected ranging bounds based on given Device names: ", err)
	}

	// Listen and store Fira ranging reports from the main DUT
	reports, err := mainDutClient.GetFiraRangingReports(ctx, &us.FiraReportRequest{
		SessionID:     sessionID,
		TimeInSeconds: 60,
	})

	// Converts CrOS_1 Mac address to int, which will serve as the key for the ranging results
	dut1Address := uwb.ByteSliceToInt(controleeReq.Params.DeviceMacAddress)

	// Get ranging summary for main DUT's ranging session with CrOS 1
	summary, err := uwb.GetRangingSessionSummary(reports.RangingResults[dut1Address], &bounds, true, false)
	s.Logf("Ranging summary: %v ", *summary)

	// Report error if insufficient percentage of Status_OK reports
	if summary.SuccessReportRate < 90 {
		s.Errorf("Ranging Session Status_OK report rate too low at %f, passing criteria is 90%%", summary.SuccessReportRate)
	}

	// Report error if an insufficient share of the reported distances fall within the acceptable boundaries of the actual distances
	if summary.DistanceStats.WithinBoundaryPercentage < uwb.DistanceMinAcceptablePercent {
		s.Errorf("Ranging session distance reports within upper bound of %d and lower bound of %d is at %f%%, which is below passing criteria of %d%%",
			bounds.DistanceUpperBound, bounds.DistanceLowerBound, summary.DistanceStats.WithinBoundaryPercentage, uwb.DistanceMinAcceptablePercent)
	}

	// Report error if an insufficient share of the reported angles fall within the acceptable boundaries of the actual distances
	if aoa {
		if summary.AzimuthStats.WithinBoundaryPercentage < uwb.AngleMinAcceptablePercent {
			s.Errorf("Ranging session azimuth reports within upper bound of %f and lower bound of %f is at %f%%, which is below passing criteria of %d%%",
				bounds.AzimuthUpperBound, bounds.AzimuthLowerBound, summary.AzimuthStats.WithinBoundaryPercentage, uwb.AngleMinAcceptablePercent)
		}
	}

	// Stops and Deinits ranging session for main_dut and CrOS 1
	for i := 0; i < 2; i++ {
		deviceClient := *s.FixtValue().(*ur.FixtData).CrosClients[i]

		// StopRanging
		stopRangingRequest := uwb.StopRangingRequest{
			SessionId: sessionID,
		}
		err = uwb.CallAndCheckOK(deviceClient.StopRanging(ctx, &stopRangingRequest))
		if err != nil {
			s.Errorf("Cros device %d StopRanging failed: %v", i, err)
		}

		// DeinitSession
		deinitRequest := uwb.DeinitSessionRequest{
			SessionId: sessionID,
		}
		err = uwb.CallAndCheckOK(deviceClient.DeinitSession(ctx, &deinitRequest))
		if err != nil {
			s.Errorf("Cros Device %d DeinitSession failed: %v", i, err)
		}
	}
}
