// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/perf"
	cbiperf "go.chromium.org/tast-tests/cros/remote/cellular/callbox/iperf"
	"go.chromium.org/tast-tests/cros/remote/cellular/callbox/manager"
	"go.chromium.org/tast-tests/cros/remote/network/iperf"
	"go.chromium.org/tast/core/testing"
)

type iperfTestCaseConfiguration struct {
	testType          cbiperf.TestType
	additionalOptions []iperf.ConfigOption
}

type iperfTestCase struct {
	callboxOpts         *manager.ConfigureCallboxRequestBody
	iperfConfigurations []iperfTestCaseConfiguration
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         IperfTx,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Conducts cellular performance tests between DUT and callbox using Iperf to compare actual throughput results with expected for a given network configuration",
		Contacts: []string{
			"chromeos-cellular-team@google.com",
			"jstanko@google.com",
		},
		BugComponent: "b:1108821",
		Attr:         []string{"group:cellular", "cellular_callbox", "cellular_cmw_callbox"},
		ServiceDeps:  []string{"tast.cros.cellular.RemoteCellularService"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "callboxManagedFixture",
		Timeout:      15 * time.Minute,
		Params: []testing.Param{{
			// Establishes a LTE connection with the callbox and runs multiple tcp iperf sessions with TCPTx configuration for uplink.
			Name: "tcp",
			Val: iperfTestCase{
				callboxOpts: &manager.ConfigureCallboxRequestBody{
					Hardware:     manager.CallboxHardwareCMW,
					CellularType: manager.CellularTechnologyLTE,
					Parameters: []manager.CellConfiguration{
						manager.NewLteCellConfiguration(
							manager.BandOption(2)),
					},
				},
				iperfConfigurations: []iperfTestCaseConfiguration{{testType: cbiperf.TestTypeTCPTx}},
			},
		}, {
			// Establishes a LTE connection with the callbox and runs multiple udp iperf sessions with UDPTx configuration for uplink.
			Name: "udp",
			Val: iperfTestCase{
				callboxOpts: &manager.ConfigureCallboxRequestBody{
					Hardware:     manager.CallboxHardwareCMW,
					CellularType: manager.CellularTechnologyLTE,
					Parameters: []manager.CellConfiguration{
						manager.NewLteCellConfiguration(
							manager.BandOption(2)),
					},
				},
				iperfConfigurations: []iperfTestCaseConfiguration{{testType: cbiperf.TestTypeUDPTx}},
			},
		},
		},
	})
}

func IperfTx(ctx context.Context, s *testing.State) {
	tc := s.Param().(iperfTestCase)
	tf := s.FixtValue().(*manager.TestFixture)
	dutConn := s.DUT().Conn()

	if err := tf.ConnectToCallbox(ctx, dutConn, tc.callboxOpts); err != nil {
		s.Fatal("Failed to initialize cellular connection: ", err)
	}

	testManager := cbiperf.NewTestManager(tf.Vars.Callbox, dutConn, tf.CallboxManagerClient)

	// Get Modem type to apply throughput values accordingly.
	resp, err := tf.RemoteCellularClient.QueryModemType(ctx, &empty.Empty{})
	if err != nil {
		s.Fatal("Failed to get modemType: ", err)
	}
	s.Log("modemType is :", resp.ModemType)
	max := manager.GetMaxLTETxThroughputInMbps(ctx, resp.ModemType)
	additionalOptions := []iperf.ConfigOption{iperf.TestTimeOption(30 * time.Second), iperf.MaxBandwidthOption(iperf.BitRate(max) * iperf.Mbps), iperf.ClientWindowSizeOption(320 * iperf.KB), iperf.ServerWindowSizeOption(320 * iperf.KB)}

	// Test is Tx/upload so DUT is client and callbox is server.
	perfValues := perf.NewValues()
	for _, config := range tc.iperfConfigurations {
		subTest := func(ctx context.Context, s *testing.State) {
			history, err := testManager.RunOnce(ctx, config.testType, tf.InterfaceName, additionalOptions)
			if err != nil {
				s.Fatal("Failed to run iperf session: ", err)
			}

			result, err := iperf.NewResultFromHistory(*history)
			if err != nil {
				s.Fatal("Failed to calculate iperf result statistics: ", err)
			}

			perfValues.Set(perf.Metric{
				Name:      string(config.testType),
				Unit:      "Mbps",
				Direction: perf.BiggerIsBetter,
			}, float64(result.Throughput/iperf.Mbps))

			min, target, err := testManager.CalculateExpectedThroughput(ctx, config.testType)
			if err != nil {
				s.Fatal("Failed to calculate expected throughput: ", err)
			}

			if result.Throughput < min {
				s.Fatalf("Throughput below required, got: %v want: %v Mbit/s", result.Throughput/iperf.Mbps, min/iperf.Mbps)
			}

			if result.Throughput < target {
				s.Logf("WARNING: Throughput below target, got: %v want: %v Mbit/s", result.Throughput/iperf.Mbps, target/iperf.Mbps)
			}

			s.Logf("Finished Iperf test %v, minimum: %v target: %v actual: %v +/- %v Mbit/s",
				config.testType, min/iperf.Mbps, target/iperf.Mbps, result.Throughput/iperf.Mbps, result.StdDeviation/iperf.Mbps)
		}

		s.Run(ctx, fmt.Sprintf("Testcase %s", config.testType), subTest)
	}

	if err := perfValues.Save(s.OutDir()); err != nil {
		s.Fatal("Failed to save perf data: ", err)
	}
}
