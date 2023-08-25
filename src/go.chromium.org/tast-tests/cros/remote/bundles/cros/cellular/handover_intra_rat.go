// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/remote/cellular/callbox/manager"
	"go.chromium.org/tast/core/testing"
)

type intraRatTestCase struct {
	startingOptions *manager.ConfigureCallboxRequestBody
	handovers       []manager.HandoverRequestBody
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         HandoverIntraRat,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that the DUT is able to perform handovers within a RAT",
		Contacts: []string{
			"chromeos-cellular-team@google.com",
			"jstanko@google.com",
		},
		BugComponent: "b:167157",
		Attr:         []string{"group:cellular", "cellular_callbox", "cellular_run_isolated", "cellular_handover"},
		ServiceDeps:  []string{"tast.cros.cellular.RemoteCellularService"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "callboxManagedFixture",
		Timeout:      10 * time.Minute,
		Params: []testing.Param{
			{
				// Separate cmw and cmx handovers as CMX requires multiple cells to be defined.
				Name:      "lte_cmw",
				ExtraAttr: []string{"cellular_cmw_callbox"},
				Val: intraRatTestCase{
					startingOptions: &manager.ConfigureCallboxRequestBody{
						CellularType: manager.CellularTechnologyLTE,
						Parameters: []manager.CellConfiguration{
							manager.NewLteCellConfiguration(
								manager.BandOption(2),
								manager.BandwidthOption(manager.Bandwidth15MHz),
							),
						},
					},
					handovers: []manager.HandoverRequestBody{
						manager.HandoverRequestBody{
							Band:      41,
							Channel:   40620,
							Bandwidth: 20,
						},
						manager.HandoverRequestBody{
							Band:      3,
							Channel:   1575,
							Bandwidth: 20,
						},
					},
				},
			},
			{
				Name:      "lte_cmx",
				ExtraAttr: []string{"cellular_cmx_callbox"},
				Val: intraRatTestCase{
					startingOptions: &manager.ConfigureCallboxRequestBody{
						CellularType: manager.CellularTechnologyLTE,
						Parameters: []manager.CellConfiguration{
							manager.NewLteCellConfiguration(
								manager.BandOption(2),
								manager.BandwidthOption(manager.Bandwidth15MHz),
							),
							manager.NewLteCellConfiguration(
								manager.BandOption(7),
								manager.BandwidthOption(manager.Bandwidth15MHz),
								manager.TrackingAreaOption(2),
							),
						},
					},
					handovers: []manager.HandoverRequestBody{
						manager.HandoverRequestBody{
							Band:      7,
							Channel:   3100,
							Bandwidth: 20,
						},
						manager.HandoverRequestBody{
							Band:      2,
							Channel:   900,
							Bandwidth: 20,
						},
					},
				},
			},
			{
				Name:      "nr5gnsa",
				ExtraAttr: []string{"cellular_cmx_callbox"},
				Val: intraRatTestCase{
					startingOptions: &manager.ConfigureCallboxRequestBody{
						CellularType: manager.CellularTechnologyNR5GSA,
						Parameters: []manager.CellConfiguration{
							manager.New5GNSACellConfiguration(
								manager.NBandOption(78),
								manager.AntennaOption(manager.MimoMode4x4, manager.TransmissionMode3),
								manager.SchedulingOption(100, 100, 4, 4),
							),
							manager.New5GNSACellConfiguration(
								manager.NBandOption(79),
								manager.AntennaOption(manager.MimoMode4x4, manager.TransmissionMode3),
								manager.TrackingAreaOption(2),
								manager.SchedulingOption(100, 100, 4, 4),
							),
						},
					},
					handovers: []manager.HandoverRequestBody{
						manager.HandoverRequestBody{
							Destination: manager.CellularTechnologyNR5GSA,
							Band:        79,
							Channel:     713334,
							Bandwidth:   100,
						},
						manager.HandoverRequestBody{
							Destination: manager.CellularTechnologyNR5GSA,
							Band:        78,
							Channel:     633696,
							Bandwidth:   100,
						},
					},
				},
			},
		},
	})
}

func HandoverIntraRat(ctx context.Context, s *testing.State) {
	tc := s.Param().(intraRatTestCase)
	tf := s.FixtValue().(*manager.TestFixture)
	dutConn := s.DUT().Conn()

	if err := tf.ConnectToCallbox(ctx, dutConn, tc.startingOptions); err != nil {
		s.Fatal("Failed to initialize cellular connection: ", err)
	}

	// cycle through the requested bands
	for _, config := range tc.handovers {
		s.Logf("Starting handover to band: %d", config.Band)
		if err := tf.CallboxManagerClient.Handover(ctx, &config); err != nil {
			s.Fatal("Failed to perform handover: ", err)
		}

		if _, _, err := tf.VerifyConnectivity(ctx); err != nil {
			s.Fatal("Failed to verify cellular connectivity: ", err)
		}
	}
}
