// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         EhideNoEthernet,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that there is no Ethernet connection when ehide has started",
		Timeout:      30 * time.Second,
		Contacts:     []string{"cros-networking@google.com", "chenzikai@google.com"},
		BugComponent: "b:1493959", // ChromeOS > Platform > baseOS > Networking > Continuous Maintenance
		Attr:         []string{"group:mainline", "informational"},
		Fixture:      "ehide",
	})
}

func EhideNoEthernet(ctx context.Context, s *testing.State) {
	m, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create a Shill Manager: ", err)
	}

	ethernetProperties := map[string]interface{}{
		shillconst.ServicePropertyType:        shillconst.TypeEthernet,
		shillconst.ServicePropertyIsConnected: true,
	}

	if service, err := m.FindMatchingService(ctx, ethernetProperties); err != nil {
		// This is what we expected.
		if err.Error() == shillconst.ErrorMatchingServiceNotFound {
			return
		}

		s.Fatal("Encountered unexpected error in finding an Ethernet service: ", err)
	} else {
		s.Fatalf("Found an Ethernet service %s, want not found", service.ObjectPath())
	}
}
