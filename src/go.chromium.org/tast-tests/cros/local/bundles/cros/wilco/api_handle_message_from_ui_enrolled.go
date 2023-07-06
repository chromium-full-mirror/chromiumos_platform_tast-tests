// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wilco

import (
	"context"
	"encoding/json"
	"time"

	dtcpb "chromiumos/wilco_dtc"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/wilco/wilcoextension"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast-tests/cros/local/wilco"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         APIHandleMessageFromUIEnrolled,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test sending a message from a Chromium extension to the Wilco DTC VM",
		Contacts:     []string{"chromeos-oem-services@google.com"},
		// ChromeOS > Software > Commercial (Enterprise) > OEM Services.
		BugComponent: "b:1256717",
		// TODO(b/274585281): Fix and reenable test.
		// Attr: []string{
		// 	"group:golden_tier",
		// 	"group:medium_low_tier",
		// 	"group:hardware",
		// 	"group:complementary",
		// },
		SoftwareDeps: []string{"reboot", "vm_host", "wilco", "chrome"},
		Fixture:      "wilcoDTCAllowedVMTestMode",
	})
}

// APIHandleMessageFromUIEnrolled tests Wilco DTC HandleMessageFromUi gRPC API.
func APIHandleMessageFromUIEnrolled(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*fixtures.FixtData).Chrome()

	type testMsg struct {
		Test int
	}

	uiRequest := testMsg{
		Test: 5,
	}
	vmResponse := testMsg{
		Test: 5,
	}

	marshaled, err := json.Marshal(vmResponse)
	if err != nil {
		s.Fatal("Failed to marshal message: ", err)
	}
	response := &dtcpb.HandleMessageFromUiResponse{
		ResponseJsonMessage: string(marshaled),
	}

	// Listening for DPSL messages.
	rec, err := wilco.NewDPSLMessageReceiver(ctx, wilco.WithHandleMessageFromUiResponse(response))
	if err != nil {
		s.Error("Failed to create dpsl message listener: ", err)
	}
	defer rec.Stop(ctx)

	// Connection to wilco test extension to send message to wilco DTC VM.
	wConn, err := wilcoextension.NewConnectionToWilcoExtension(ctx, cr)
	if err != nil {
		s.Fatal("Failed to create connection to extension: ", err)
	}
	defer wConn.CloseTarget(ctx)
	defer wConn.Close()

	if err := wConn.CreatePort(ctx); err != nil {
		s.Fatal("Failed to create port to built-in application: ", err)
	}

	s.Log("Sending message from extension")
	sendMessageCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var uiResponse testMsg
	if err := wConn.SendMessageAndGetReply(sendMessageCtx, &uiRequest, &uiResponse); err != nil {
		s.Fatal("Failed to send message using extension: ", err)
	}
	if uiResponse != vmResponse {
		s.Errorf("Unexpected response received = got %v, want %v", uiResponse, vmResponse)
	}

	s.Log("Waiting for HandleMessageFromUi")
	eventCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	msg := dtcpb.HandleMessageFromUiRequest{}
	if err := rec.WaitForMessage(eventCtx, &msg); err != nil {
		s.Fatal("Failed to receive HandleMessageFromUi event: ", err)
	}

	var vmRequest testMsg
	if err := json.Unmarshal([]byte(msg.JsonMessage), &vmRequest); err != nil {
		s.Fatalf("Failed to unmarshall %q: %v", msg.JsonMessage, err)
	}

	if uiRequest != vmRequest {
		s.Errorf("Unexpected message received = got %v, want %v", vmRequest, uiRequest)
	}
}
