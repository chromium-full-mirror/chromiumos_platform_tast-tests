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

type iperfTestCaseConfiguration1 struct {
	testType          cbiperf.TestType
	additionalOptions []iperf.ConfigOption
}

type iperfTestCase1 struct {
	iperfConfigurations []iperfTestCaseConfiguration1
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         IperfRx,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Conducts cellular performance tests between DUT and callbox using Iperf to compare actual throughput results with expected for a given network configuration",
		Contacts:     []string{"chromeos-cellular-team@google.com", "srikanthkumar@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_callbox", "cellular_cmw_callbox"},
		ServiceDeps:  []string{"tast.cros.cellular.RemoteCellularService"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "callboxManagedFixture",
		Timeout:      15 * time.Minute,
		Params: []testing.Param{
			{
				// Establishes a LTE connection with the callbox and runs multiple tcp iperf sessions with TCPRx configuration for downlink.
				Name: "tcp",
				Val:  iperfTestCase1{iperfConfigurations: []iperfTestCaseConfiguration1{{testType: cbiperf.TestTypeTCPRx}}},
			}, {
				// Establishes a LTE connection with the callbox and runs multiple tcp iperf sessions with UDPRx configuration for downlink.
				Name: "udp",
				Val:  iperfTestCase1{iperfConfigurations: []iperfTestCaseConfiguration1{{testType: cbiperf.TestTypeUDPRx}}},
			},
		},
	})
}

func IperfRx(ctx context.Context, s *testing.State) {
	tc := s.Param().(iperfTestCase1)
	tf := s.FixtValue().(*manager.TestFixture)
	dutConn := s.DUT().Conn()
	// Get Modem type to apply throughput values accordingly.
	resp, err := tf.RemoteCellularClient.QueryModemType(ctx, &empty.Empty{})
	if err != nil {
		s.Fatal("Failed to get modemType: ", err)
	}
	s.Log("modemType is :", resp.ModemType)
	max := manager.GetMaxLTERxThroughputInMbps(ctx, resp.ModemType)
	// Run twice with two types of CA modes if modem supports!.
	config := manager.GetCellConfiguration(ctx, manager.CallboxHardwareCMW, resp.ModemType)
	if err := tf.ConnectToCallbox(ctx, dutConn, config); err != nil {
		s.Fatal("Failed to initialize cellular connection: ", err)
	}

	testManager := cbiperf.NewTestManager(tf.Vars.Callbox, dutConn, tf.CallboxManagerClient)

	additionalOptions := []iperf.ConfigOption{iperf.TestTimeOption(20 * time.Second), iperf.MaxBandwidthOption(iperf.BitRate(max) * iperf.Mbps), iperf.ClientWindowSizeOption(1.4 * iperf.MB), iperf.ServerWindowSizeOption(1.4 * iperf.MB)}

	// Test is Rx/Download so DUT is server and callbox is client.
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
