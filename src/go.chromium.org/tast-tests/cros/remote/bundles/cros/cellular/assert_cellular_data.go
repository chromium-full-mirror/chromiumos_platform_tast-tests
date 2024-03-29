// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/remote/cellular/callbox/manager"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/cellularconst"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AssertCellularData,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that the DUT is able to establish a data connection under various RAT scenarios",
		Contacts: []string{
			"chromeos-cellular-team@google.com",
		},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_callbox"},
		ServiceDeps:  []string{"tast.cros.cellular.RemoteCellularService"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "callboxManagedFixture",
		Timeout:      7 * time.Minute,
		Params: []testing.Param{
			{
				// names are defined <tech>_<#CCs>_<MIMO>_<CA BANDS>
				Name:      "lte_cc1_x1_b3",
				ExtraAttr: []string{"cellular_cmw_callbox", "cellular_cmx_callbox"},
				Val: &manager.ConfigureCallboxRequestBody{
					CellularType: manager.CellularTechnologyLTE,
					Parameters: []manager.CellConfiguration{
						manager.NewLteCellConfiguration(
							manager.BandOption(3),
							manager.AntennaOption(manager.MimoMode1x1, manager.TransmissionMode1),
						),
					},
				},
			},
			{
				Name:      "lte_cc1_x2_b3",
				ExtraAttr: []string{"cellular_cmw_callbox", "cellular_cmx_callbox"},
				Val: &manager.ConfigureCallboxRequestBody{
					CellularType: manager.CellularTechnologyLTE,
					Parameters: []manager.CellConfiguration{
						manager.NewLteCellConfiguration(
							manager.BandOption(3),
							manager.AntennaOption(manager.MimoMode2x2, manager.TransmissionMode3),
						),
					},
				},
			},
			{
				Name:      "lte_cc2_x1x1_b3b7",
				ExtraAttr: []string{"cellular_cmw_callbox", "cellular_cmx_callbox"},
				Val: &manager.ConfigureCallboxRequestBody{
					CellularType: manager.CellularTechnologyLTE,
					Parameters: []manager.CellConfiguration{
						manager.NewLteCellConfiguration(
							manager.BandOption(3),
							manager.AntennaOption(manager.MimoMode1x1, manager.TransmissionMode1),
						),
						manager.NewLteCellConfiguration(
							manager.BandOption(7),
							manager.AntennaOption(manager.MimoMode1x1, manager.TransmissionMode1),
						),
					},
				},
			},
			{
				Name: "lte_cc3_x1x1x1_b3b7b20",
				// 3CA not supported on FM101 or EM060.
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnCellularModemType(
					cellularconst.ModemTypeFM101, cellularconst.ModemTypeEM060),
				),
				ExtraAttr: []string{"cellular_cmw_callbox", "cellular_cmx_callbox"},
				Val: &manager.ConfigureCallboxRequestBody{
					CellularType: manager.CellularTechnologyLTE,
					Parameters: []manager.CellConfiguration{
						manager.NewLteCellConfiguration(
							manager.BandOption(3),
							manager.AntennaOption(manager.MimoMode1x1, manager.TransmissionMode1),
						),
						manager.NewLteCellConfiguration(
							manager.BandOption(7),
							manager.AntennaOption(manager.MimoMode1x1, manager.TransmissionMode1),
						),
						manager.NewLteCellConfiguration(
							manager.BandOption(20),
							manager.AntennaOption(manager.MimoMode1x1, manager.TransmissionMode1),
						),
					},
				},
			},
			{
				Name: "lte_cc1_x4_b3",
				// 4x4 lte is only supported on CMX
				ExtraAttr: []string{"cellular_cmx_callbox"},
				Val: &manager.ConfigureCallboxRequestBody{
					CellularType: manager.CellularTechnologyLTE,
					Parameters: []manager.CellConfiguration{
						manager.NewLteCellConfiguration(
							manager.BandOption(1),
							manager.AntennaOption(manager.MimoMode4x4, manager.TransmissionMode3),
						),
					},
				},
			},
			{
				Name:      "lte_cc2_x4x4_b3b7",
				ExtraAttr: []string{"cellular_cmx_callbox"},
				Val: &manager.ConfigureCallboxRequestBody{
					CellularType: manager.CellularTechnologyLTE,
					Parameters: []manager.CellConfiguration{
						manager.NewLteCellConfiguration(
							manager.BandOption(3),
							manager.AntennaOption(manager.MimoMode4x4, manager.TransmissionMode3),
						),
						manager.NewLteCellConfiguration(
							manager.BandOption(7),
							manager.AntennaOption(manager.MimoMode4x4, manager.TransmissionMode3),
						),
					},
				},
			},
			{
				Name:      "nr5gnsa_cc2_x4x4_b1n78",
				ExtraAttr: []string{"cellular_cmx_callbox"},
				Val: &manager.ConfigureCallboxRequestBody{
					CellularType: manager.CellularTechnologyNR5GNSA,
					Parameters: []manager.CellConfiguration{
						manager.NewLteCellConfiguration(
							manager.BandOption(1),
							manager.AntennaOption(manager.MimoMode4x4, manager.TransmissionMode3),
						),
						manager.New5GNSACellConfiguration(
							manager.NBandOption(78),
							manager.AntennaOption(manager.MimoMode4x4, manager.TransmissionMode3),
						),
					},
				},
			},
			{
				Name:      "nr5gnsa_cc3_x4x4x4_b3b1n78",
				ExtraAttr: []string{"cellular_cmx_callbox"},
				Val: &manager.ConfigureCallboxRequestBody{
					CellularType: manager.CellularTechnologyNR5GNSA,
					Parameters: []manager.CellConfiguration{
						manager.NewLteCellConfiguration(
							manager.BandOption(3),
							manager.AntennaOption(manager.MimoMode4x4, manager.TransmissionMode3),
						),
						manager.NewLteCellConfiguration(
							manager.BandOption(1),
							manager.AntennaOption(manager.MimoMode4x4, manager.TransmissionMode3),
						),
						manager.New5GNSACellConfiguration(
							manager.NBandOption(78),
							manager.AntennaOption(manager.MimoMode4x4, manager.TransmissionMode3),
						),
					},
				},
			},
			{
				Name:      "nr5gnsa_cc4_x4x4x4x4_b3b1b7n78",
				ExtraAttr: []string{"cellular_cmx_callbox"},
				Val: &manager.ConfigureCallboxRequestBody{
					CellularType: manager.CellularTechnologyNR5GNSA,
					Parameters: []manager.CellConfiguration{
						manager.NewLteCellConfiguration(
							manager.BandOption(3),
							manager.AntennaOption(manager.MimoMode4x4, manager.TransmissionMode3),
						),
						manager.NewLteCellConfiguration(
							manager.BandOption(1),
							manager.AntennaOption(manager.MimoMode4x4, manager.TransmissionMode3),
						),
						manager.NewLteCellConfiguration(
							manager.BandOption(7),
							manager.AntennaOption(manager.MimoMode4x4, manager.TransmissionMode3),
						),
						manager.New5GNSACellConfiguration(
							manager.NBandOption(78),
							manager.AntennaOption(manager.MimoMode4x4, manager.TransmissionMode3),
						),
					},
				},
			},
			{
				Name:      "nr5gnsa_cc3_x4x4x4_b1n78n78",
				ExtraAttr: []string{"cellular_cmx_callbox"},
				Val: &manager.ConfigureCallboxRequestBody{
					CellularType: manager.CellularTechnologyNR5GNSA,
					Parameters: []manager.CellConfiguration{
						manager.NewLteCellConfiguration(
							manager.BandOption(1),
							manager.AntennaOption(manager.MimoMode4x4, manager.TransmissionMode3),
						),
						manager.New5GNSACellConfiguration(
							manager.NBandOption(78),
							manager.NRARFCNOption(630942),
							manager.AntennaOption(manager.MimoMode4x4, manager.TransmissionMode3),
						),
						manager.New5GNSACellConfiguration(
							manager.NBandOption(78),
							manager.NRARFCNOption(640054),
							manager.BandwidthOption(manager.Bandwidth20MHz),
							manager.AntennaOption(manager.MimoMode4x4, manager.TransmissionMode3),
						),
					},
				},
			},
			{
				Name:      "nr5gsa_cc1_x4_n78",
				ExtraAttr: []string{"cellular_cmx_callbox"},
				Val: &manager.ConfigureCallboxRequestBody{
					CellularType: manager.CellularTechnologyNR5GSA,
					Parameters: []manager.CellConfiguration{
						manager.New5GNSACellConfiguration(
							manager.NBandOption(78),
							manager.AntennaOption(manager.MimoMode4x4, manager.TransmissionMode3),
						),
					},
				},
			},
			{
				Name:      "nr5gsa_cc2_x4x4_n78n78",
				ExtraAttr: []string{"cellular_cmx_callbox"},
				Val: &manager.ConfigureCallboxRequestBody{
					CellularType: manager.CellularTechnologyNR5GSA,
					Parameters: []manager.CellConfiguration{
						manager.New5GNSACellConfiguration(
							manager.NBandOption(78),
							manager.RxPowerOption(manager.NewRxPower(-70)),
							manager.NRARFCNOption(630942),
							manager.AntennaOption(manager.MimoMode4x4, manager.TransmissionMode3),
						),
						manager.New5GNSACellConfiguration(
							manager.NBandOption(78),
							manager.RxPowerOption(manager.NewRxPower(-70)),
							manager.NRARFCNOption(640054),
							manager.AntennaOption(manager.MimoMode4x4, manager.TransmissionMode3),
						),
					},
				},
			},
		},
	})
}

func AssertCellularData(ctx context.Context, s *testing.State) {
	dutConn := s.DUT().Conn()
	tf := s.FixtValue().(*manager.TestFixture)
	tc := s.Param().(*manager.ConfigureCallboxRequestBody)
	// ConnectToCallbox will already verify IP connectivity as part of the test
	// so no additional check is needed.
	if err := tf.ConnectToCallbox(ctx, dutConn, tc); err != nil {
		s.Fatal("Failed to initialize cellular connection: ", err)
	}
}
