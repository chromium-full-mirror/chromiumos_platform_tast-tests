// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/remote/cellular/callbox/manager"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type interRatTestCase struct {
	startingOptions *manager.ConfigureCallboxRequestBody
	handovers       []manager.HandoverRequestBody
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         HandoverInterRat,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that the DUT is able to handover between two RATs",
		Contacts: []string{
			"chromeos-cellular-team@google.com",
			"jstanko@google.com",
		},
		BugComponent: "b:167157",
		Attr:         []string{"group:cellular", "cellular_callbox", "cellular_run_isolated", "cellular_handover"},
		ServiceDeps:  []string{"tast.cros.cellular.RemoteCellularService"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "callboxManagedFixture",
		Timeout:      12 * time.Minute,
		Params: []testing.Param{
			{
				Name:      "lte_wcdma",
				ExtraAttr: []string{"cellular_cmw_callbox"},
				Val: interRatTestCase{
					startingOptions: &manager.ConfigureCallboxRequestBody{
						CellularType: manager.CellularTechnologyLTE,
						Parameters: []manager.CellConfiguration{
							manager.NewLteCellConfiguration(
								manager.BandOption(2),
								manager.BandwidthOption(manager.Bandwidth20MHz),
							),
						},
					},
					handovers: []manager.HandoverRequestBody{
						manager.HandoverRequestBody{
							Destination: manager.CellularTechnologyWCDMA,
							Band:        5,
							Channel:     4357,
						},
						manager.HandoverRequestBody{
							Destination: manager.CellularTechnologyWCDMA,
							Band:        8,
							Channel:     2937,
						},
						manager.HandoverRequestBody{
							Destination: manager.CellularTechnologyLTE,
							Band:        3,
							Channel:     1575,
							Bandwidth:   15,
						},
					},
				},
			},
			{
				Name:      "nsa_lte",
				ExtraAttr: []string{"cellular_cmx_callbox"},
				Val: interRatTestCase{
					startingOptions: &manager.ConfigureCallboxRequestBody{
						CellularType: manager.CellularTechnologyNR5GNSA,
						Parameters: []manager.CellConfiguration{
							manager.NewLteCellConfiguration(
								manager.BandOption(1),
								manager.AntennaOption(manager.MimoMode4x4, manager.TransmissionMode3),
							),
							manager.New5GNSACellConfiguration(
								manager.NBandOption(78),
								manager.SchedulingOption(100, 100, 4, 4),
								manager.AntennaOption(manager.MimoMode4x4, manager.TransmissionMode3),
							),
							manager.NewLteCellConfiguration(
								manager.BandOption(3),
								manager.AntennaOption(manager.MimoMode4x4, manager.TransmissionMode3),
								manager.TrackingAreaOption(2),
							),
						},
					},
					handovers: []manager.HandoverRequestBody{
						manager.HandoverRequestBody{
							Destination: manager.CellularTechnologyLTE,
							Band:        3,
							Channel:     1575,
							Bandwidth:   15,
						},
						manager.HandoverRequestBody{
							Destination:        manager.CellularTechnologyNR5GNSA,
							Band:               1,
							Channel:            300,
							Bandwidth:          20,
							SecondaryBand:      78,
							SecondaryChannel:   633696,
							SecondaryBandwidth: 20,
						},
					},
				},
			},
			{
				// NSA -> NSA is technically Inter-RAT since it's really NSA -> LTE -> NSA
				Name:      "nsa_nsa",
				ExtraAttr: []string{"cellular_cmx_callbox"},
				Val: interRatTestCase{
					startingOptions: &manager.ConfigureCallboxRequestBody{
						CellularType: manager.CellularTechnologyNR5GNSA,
						Parameters: []manager.CellConfiguration{
							manager.NewLteCellConfiguration(
								manager.BandOption(1),
								manager.SchedulingOption(50, 50, 4, 4),
								manager.AntennaOption(manager.MimoMode4x4, manager.TransmissionMode3),
							),
							manager.New5GNSACellConfiguration(
								manager.NBandOption(78),
								manager.SchedulingOption(100, 100, 4, 4),
								manager.AntennaOption(manager.MimoMode4x4, manager.TransmissionMode3),
							),
							manager.NewLteCellConfiguration(
								manager.BandOption(1),
								manager.SchedulingOption(50, 50, 4, 4),
								manager.AntennaOption(manager.MimoMode4x4, manager.TransmissionMode3),
								manager.TrackingAreaOption(2),
							),
							manager.New5GNSACellConfiguration(
								manager.NBandOption(78),
								manager.SchedulingOption(100, 100, 4, 4),
								manager.AntennaOption(manager.MimoMode4x4, manager.TransmissionMode3),
								manager.TrackingAreaOption(2),
							),
						},
					},
					handovers: []manager.HandoverRequestBody{
						manager.HandoverRequestBody{
							Destination:        manager.CellularTechnologyNR5GNSA,
							Band:               1,
							Channel:            300,
							Bandwidth:          20,
							SecondaryBand:      78,
							SecondaryChannel:   633696,
							SecondaryBandwidth: 20,
						},
						manager.HandoverRequestBody{
							Destination:        manager.CellularTechnologyNR5GNSA,
							Band:               1,
							Channel:            300,
							Bandwidth:          20,
							SecondaryBand:      78,
							SecondaryChannel:   633696,
							SecondaryBandwidth: 20,
						},
					},
				},
			},
			{
				Name:      "nr5gsa_lte",
				ExtraAttr: []string{"cellular_cmx_callbox"},
				Val: interRatTestCase{
					startingOptions: &manager.ConfigureCallboxRequestBody{
						CellularType: manager.CellularTechnologyNR5GSA,
						Parameters: []manager.CellConfiguration{
							manager.New5GNSACellConfiguration(
								manager.NBandOption(78),
								manager.SchedulingOption(100, 100, 4, 4),
							),
							manager.NewLteCellConfiguration(
								manager.BandOption(3),
								manager.SchedulingOption(50, 50, 4, 4),
								manager.TrackingAreaOption(2),
							),
						},
					},
					handovers: []manager.HandoverRequestBody{
						manager.HandoverRequestBody{
							Destination: manager.CellularTechnologyLTE,
							Band:        3,
							Channel:     1575,
							Bandwidth:   15,
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

var ratMap = map[manager.CellularTechnology]string{
	manager.CellularTechnologyLTE:     shillconst.CellularNetworkTechnologyLTE,
	manager.CellularTechnologyWCDMA:   shillconst.CellularNetworkTechnologyUMTS,
	manager.CellularTechnologyNR5GNSA: shillconst.CellularNetworkTechnology5GNR,
	manager.CellularTechnologyNR5GSA:  shillconst.CellularNetworkTechnology5GNR,
}

func HandoverInterRat(ctx context.Context, s *testing.State) {
	tc := s.Param().(interRatTestCase)
	tf := s.FixtValue().(*manager.TestFixture)
	dutConn := s.DUT().Conn()

	if err := tf.ConnectToCallbox(ctx, dutConn, tc.startingOptions); err != nil {
		s.Fatal("Failed to initialize cellular connection: ", err)
	}

	// cycle through the requested RATs
	for _, config := range tc.handovers {
		s.Logf("Starting handover to technology: %s, band: %d", config.Destination, config.Band)
		if err := tf.CallboxManagerClient.Handover(ctx, &config); err != nil {
			s.Fatal("Failed to perform handover: ", err)
		}

		// verify network type
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			resp, err := tf.RemoteCellularClient.QueryService(ctx, &empty.Empty{})
			if err != nil {
				return errors.Wrap(err, "failed to get cellular service details")
			}

			if ratMap[config.Destination] != resp.NetworkTechnology {
				return errors.Errorf("unexpected network technology: got %q, want %q", resp.NetworkTechnology, config.Destination)
			}
			return nil
		}, &testing.PollOptions{Interval: time.Second, Timeout: time.Minute}); err != nil {
			s.Fatal("Failed to verify network technology: ", err)
		}

		// make sure we're connected to the network and not just attached if handover was a redirect
		if resp, err := tf.RemoteCellularClient.QueryService(ctx, &empty.Empty{}); err != nil {
			s.Fatal("Failed to get cellular service details: ", err)
		} else if !resp.IsConnected {
			if _, err := tf.RemoteCellularClient.Connect(ctx, &empty.Empty{}); err != nil {
				s.Fatal("Failed to connect to cellular network: ", err)
			}
		}

		if _, _, err := tf.VerifyConnectivity(ctx); err != nil {
			s.Fatal("Failed to verify cellular connectivity: ", err)
		}
	}
}
