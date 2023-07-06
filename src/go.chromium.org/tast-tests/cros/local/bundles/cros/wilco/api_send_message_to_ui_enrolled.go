// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wilco

import (
	"context"
	"encoding/json"

	dtcpb "chromiumos/wilco_dtc"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/wilco/wilcoextension"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast-tests/cros/local/wilco"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         APISendMessageToUIEnrolled,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test sending a message from the Wilco DTC VM to the Chromium extension",
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
		Fixture:      "wilcoDTCAllowed",
	})
}

// APISendMessageToUIEnrolled tests Wilco DTC SendMessageToUi gRPC API.
func APISendMessageToUIEnrolled(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*fixtures.FixtData).Chrome()

	wConn, err := wilcoextension.NewConnectionToWilcoExtension(ctx, cr)
	if err != nil {
		s.Fatal("Failed to create connection to extension: ", err)
	}
	defer wConn.CloseTarget(ctx)
	defer wConn.Close()

	if err := wConn.CreatePort(ctx); err != nil {
		s.Fatal("Failed to create port to built-in application: ", err)
	}
	if err := wConn.StartListener(ctx); err != nil {
		s.Fatal("Failed to start listener: ", err)
	}

	type testMsg struct {
		Test int
	}

	uiResponse := testMsg{
		Test: 8,
	}

	if err := wConn.AddReply(ctx, &uiResponse); err != nil {
		s.Fatal("Failed to set reply: ", err)
	}

	vmRequest := testMsg{
		Test: 5,
	}
	marshaled, err := json.Marshal(vmRequest)
	if err != nil {
		s.Fatal("Failed to marshal message: ", err)
	}

	s.Log("Sending message to extension")
	request := dtcpb.SendMessageToUiRequest{
		JsonMessage: string(marshaled),
	}
	response := dtcpb.SendMessageToUiResponse{}

	if err := wilco.DPSLSendMessage(ctx, "SendMessageToUi", &request, &response); err != nil {
		s.Error("Failed to send message to UI: ", err)
	}

	var vmResponse testMsg
	if err := json.Unmarshal([]byte(response.ResponseJsonMessage), &vmResponse); err != nil {
		s.Logf("Response JSON message: %q", response.ResponseJsonMessage)
		s.Fatal("Failed to unmarshal message: ", err)
	}

	if uiResponse != vmResponse {
		s.Errorf("Unexpected reply received = got %v, want %v", vmResponse, uiResponse)
	}

	s.Log("Waiting for message")
	var uiRequest testMsg
	if err := wConn.WaitForMessage(ctx, &uiRequest); err != nil {
		s.Fatal("Failed to wait for a message received by extension: ", err)
	}

	if vmRequest != uiRequest {
		s.Errorf("Unexpected request received = got %v, want %v", uiRequest, vmRequest)
	}
}
