// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wilco

import (
	"context"

	dtcpb "chromiumos/wilco_dtc"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/wilco/pre"
	"go.chromium.org/tast-tests/cros/local/wilco"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     APIGetOsVersion,
		Desc:     "Test sending GetOsVersion gRPC request from Wilco DTC VM to the Wilco DTC Support Daemon",
		Contacts: []string{"chromeos-oem-services@google.com"},
		// ChromeOS > Software > Commercial (Enterprise) > OEM Services.
		BugComponent: "b:1256717",
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"vm_host", "wilco"},
		Pre:          pre.WilcoDtcSupportdAPI,
	})
}

func APIGetOsVersion(ctx context.Context, s *testing.State) {
	request := dtcpb.GetOsVersionRequest{}
	response := dtcpb.GetOsVersionResponse{}

	if err := wilco.DPSLSendMessage(ctx, "GetOsVersion", &request, &response); err != nil {
		s.Fatal("Unable to get OS version: ", err)
	}

	// Error conditions defined by the proto definition.
	if len(response.Version) == 0 {
		s.Fatal(errors.Errorf("OS Version is blank: %s", response.String()))
	}
	if response.Milestone == 0 {
		s.Fatal(errors.Errorf("OS Milestone is 0: %s", response.String()))
	}
}
