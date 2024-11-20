// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/remote/cellular/callbox/manager"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AssertSMS,
		Desc:         "Verifies that an SMS message sent from the callbox is received",
		Contacts:     []string{"chromeos-cellular-team@google.com", "jstanko@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_callbox"},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.cellular.RemoteCellularService"},
		Fixture:      "callboxManagedFixture",
		Timeout:      5 * time.Minute,
		Params: []testing.Param{
			{
				Name:      "lte",
				ExtraAttr: []string{"cellular_cmw_callbox", "cellular_cmx_callbox"},
				Val: &manager.ConfigureCallboxRequestBody{
					CellularType: manager.CellularTechnologyLTE,
					Parameters: []manager.CellConfiguration{
						manager.NewLteCellConfiguration(),
					},
				},
			},
			{
				Name:      "nr5g_nsa",
				ExtraAttr: []string{"cellular_cmx_callbox"},
				// TODO(b/273954565): remove hwdep once drone push has landed so R&S carrier names can be used
				ExtraHardwareDeps: hwdep.D(hwdep.Model("vell")),
				Val: &manager.ConfigureCallboxRequestBody{
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
	})
}

func AssertSMS(ctx context.Context, s *testing.State) {
	dutConn := s.DUT().Conn()
	tf := s.FixtValue().(*manager.TestFixture)
	tc := s.Param().(*manager.ConfigureCallboxRequestBody)
	if err := tf.ConnectToCallbox(ctx, dutConn, tc); err != nil {
		s.Fatal("Failed to initialize cellular connection: ", err)
	}

	// start goroutine to watch for an SMS message on the DUT
	errorCh := make(chan error, 1)
	messageCh := make(chan string, 1)
	go func() {
		defer func() {
			close(errorCh)
			close(messageCh)
		}()
		if resp, err := tf.RemoteCellularClient.WaitForNextSms(ctx, &empty.Empty{}); err != nil {
			errorCh <- err
		} else {
			messageCh <- resp.Message.Text
		}
	}()

	// Send a message and poll until it is received on the DUT
	attempts := 0
	testMessage := "Hello " + time.Now().Format(time.UnixDate)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		attempts++
		testing.ContextLogf(ctx, "Sending SMS, attempt %d", attempts)
		if err := tf.CallboxManagerClient.SendSms(ctx, &manager.SendSmsRequestBody{Message: testMessage}); err != nil {
			s.Fatal("Failed to send SMS from callbox: ", err)
		}

		// poll for WaitForNextSMS to exit
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if len(messageCh) != 0 || len(errorCh) != 0 {
				return nil
			}
			return errors.Errorf("failed to wait for DUT to receive SMS %q", testMessage)
		}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
			return err
		}

		return nil
	}, &testing.PollOptions{}); err != nil {
		s.Fatal("Failed to wait for SMS on DUT: ", <-errorCh)
	}

	if len(errorCh) > 0 {
		s.Fatal("Failed to receive SMS message on DUT: ", <-errorCh)
	}

	receivedMessage := <-messageCh
	if testMessage != receivedMessage {
		s.Fatalf("Failed to check SMS message, got %q, expected %q", receivedMessage, testMessage)
	}
}
