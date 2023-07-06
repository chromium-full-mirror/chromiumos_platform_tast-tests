// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wilco

import (
	"context"

	dtcpb "chromiumos/wilco_dtc"

	"go.chromium.org/tast-tests/cros/local/wilco"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         APIPerformWebRequestEnrolled,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test sending PerformWebRequest to the Wilco DTC Support Daemon",
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

// APIPerformWebRequestEnrolled tests PerformWebRequest gRPC API.
func APIPerformWebRequestEnrolled(ctx context.Context, s *testing.State) {
	for _, tc := range []struct {
		name       string
		url        string
		httpMethod dtcpb.PerformWebRequestParameter_HttpMethod
		wantStatus dtcpb.PerformWebRequestResponse_Status
		// Don't fail the test as the website may not be reachable from DUT. Instead, just put a log.
		logOnly bool
	}{
		{
			name:       "google",
			url:        "https://google.com",
			httpMethod: dtcpb.PerformWebRequestParameter_HTTP_METHOD_GET,
			wantStatus: dtcpb.PerformWebRequestResponse_STATUS_OK,
			logOnly:    true,
		},
		{
			name:       "localhost",
			url:        "https://localhost/test",
			httpMethod: dtcpb.PerformWebRequestParameter_HTTP_METHOD_GET,
			// Requests to localhost are blocked.
			wantStatus: dtcpb.PerformWebRequestResponse_STATUS_NETWORK_ERROR,
		},
	} {
		s.Run(ctx, tc.name, func(ctx context.Context, s *testing.State) {
			response := dtcpb.PerformWebRequestResponse{}

			if err := wilco.DPSLSendMessage(ctx, "PerformWebRequest", &dtcpb.PerformWebRequestParameter{
				HttpMethod: tc.httpMethod,
				Url:        tc.url,
			}, &response); err != nil {
				s.Fatal("Failed to call PerformWebRequest API method: ", err)
			}

			if response.Status != tc.wantStatus {
				if tc.logOnly {
					s.Logf("Request for %s failed with invalid status = got %v, expect %v", tc.url, response.Status, tc.wantStatus)
					return
				}

				s.Errorf("Request for %s failed with invalid status = got %v, expect %v", tc.url, response.Status, tc.wantStatus)
			}
		})
	}
}
