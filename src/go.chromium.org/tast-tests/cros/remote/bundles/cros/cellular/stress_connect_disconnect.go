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

type stressConnectTestCase struct {
	ExpectedTechnology string
	callboxOpts        *manager.ConfigureCallboxRequestBody
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         StressConnectDisconnect,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that the DUT is able to connect and disconnect on 5GNSA",
		Contacts:     []string{"chromeos-cellular-team@google.com", "srikanthkumar@google.com"},
		BugComponent: "b:167157",
		Attr:         []string{"group:cellular", "cellular_callbox", "cellular_cmx_callbox"},
		ServiceDeps:  []string{"tast.cros.cellular.RemoteCellularService"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "callboxManagedFixture",
		Timeout:      12 * time.Minute,
		Params: []testing.Param{
			{
				Name:      "nr5gnsa",
				ExtraAttr: []string{"cellular_cmx_callbox"},
				Val: stressConnectTestCase{
					ExpectedTechnology: string(shillconst.CellularNetworkTechnology5GNR),
					callboxOpts: &manager.ConfigureCallboxRequestBody{
						Hardware:     manager.CallboxHardwareCMX,
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
			},
		},
	})
}

func StressConnectDisconnect(ctx context.Context, s *testing.State) {
	dutConn := s.DUT().Conn()
	tf := s.FixtValue().(*manager.TestFixture)
	tc := s.Param().(stressConnectTestCase)
	if err := tf.ConnectToCallbox(ctx, dutConn, tc.callboxOpts); err != nil {
		s.Fatal("Failed to initialize cellular connection: ", err)
	}

	// cycle through connect,disconnect.
	for i := 0; i < 10; i++ {
		s.Log("Iteration ", i)
		// Make sure we're connected to the network service.
		if resp, err := tf.RemoteCellularClient.QueryService(ctx, &empty.Empty{}); err != nil {
			s.Fatal("Failed to get cellular service details: ", err)
		} else if !resp.IsConnected {
			if _, err := tf.RemoteCellularClient.Connect(ctx, &empty.Empty{}); err != nil {
				s.Fatal("Failed to connect to cellular network: ", err)
			}
		}
		// Verify network type.
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			resp, err := tf.RemoteCellularClient.QueryService(ctx, &empty.Empty{})
			if err != nil {
				return errors.Wrap(err, "failed to get cellular service details")
			}
			if tc.ExpectedTechnology != resp.NetworkTechnology {
				return errors.Errorf("unexpected network technology: got %q, want %q", resp.NetworkTechnology, manager.CellularTechnologyNR5GNSA)
			}
			return nil
		}, &testing.PollOptions{Interval: time.Second, Timeout: time.Minute}); err != nil {
			s.Fatal("Failed to verify network technology: ", err)
		}
		// Check connection.
		if err := dutConn.CommandContext(ctx, "curl", "--interface", tf.InterfaceName, "google.com").Run(); err != nil {
			s.Fatal("Failed to curl google.com on DUT using cellular interface: ", err)
		}
		// Disconnect.
		if resp, err := tf.RemoteCellularClient.QueryService(ctx, &empty.Empty{}); err != nil {
			s.Fatal("Failed to get cellular service details: ", err)
		} else if resp.IsConnected {
			if _, err := tf.RemoteCellularClient.Disconnect(ctx, &empty.Empty{}); err != nil {
				s.Fatal("Failed to disconnect to cellular network: ", err)
			}
		}
	}
}
