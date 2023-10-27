// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"sync"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           ShillDisableWhileConnecting,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Verifies that the modem can handle being disabled while connecting in Shill",
		Contacts:       []string{"chromeos-cellular-team@google.com", "ejcaruso@google.com"},
		BugComponent:   "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:           []string{"group:cellular", "cellular_unstable", "cellular_amari_callbox"},
		HardwareDeps:   hwdep.D(hwdep.Cellular()),
		Timeout:        3 * time.Minute,
		Fixture:        "cellular",
	})
}

func ShillDisableWhileConnecting(ctx context.Context, s *testing.State) {
	helper := s.FixtValue().(*cellular.FixtData).Helper

	// Disable AutoConnect so when we enable the cellular device the
	// default service does not automatically connect.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()
	if wasAutoConnect, err := helper.SetServiceAutoConnect(ctx, false); err != nil {
		s.Fatal("Failed to disable AutoConnect: ", err)
	} else if wasAutoConnect {
		defer func(ctx context.Context) {
			if _, err := helper.SetServiceAutoConnect(ctx, true); err != nil {
				s.Fatal("Failed to enable AutoConnect: ", err)
			}
		}(cleanupCtx)
	}

	// Always attempt to re-enable cellular and connect back to the default
	// service to ensure that the test has left the DUT in a good state.
	defer func(ctx context.Context) {
		if _, err := helper.Enable(ctx); err != nil {
			s.Fatal("Failed to re-enable device before finishing: ", err)
		}
		if _, err := helper.ConnectToDefault(ctx); err != nil {
			s.Fatal("Failed to re-connect to service before finishing: ", err)
		}
	}(cleanupCtx)

	// Simply disconnecting the service here causes the later connection attempt to succeed too
	// quickly for the test to catch it with the disable. Instead, disable and enable the device.
	s.Log("Toggling device")
	if _, err := helper.Disable(ctx); err != nil {
		s.Fatal("Failed to disable device: ", err)
	}
	if _, err := helper.Enable(ctx); err != nil {
		s.Fatal("Failed to enable device: ", err)
	}

	service, err := helper.FindServiceForDevice(ctx)
	if err != nil {
		s.Fatal("Could not find service for device: ", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		// Wait for service to move to the "Associating" or "Configuring" state. This means
		// the service has started connecting.
		s.Log("Waiting for service to start connecting")
		states := []interface{}{shillconst.ServiceStateAssociation, shillconst.ServiceStateConfiguration}
		if err := service.WaitForPropertyInSetWithOptions(ctx, shillconst.ServicePropertyState, states, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 20 * time.Millisecond}); err != nil {
			s.Fatal("Service never started connecting: ", err)
		}

		// Now we can disable the device.
		s.Log("Disabling device")
		if _, err := helper.Disable(ctx); err != nil {
			s.Fatal("Failed to disable the device: ", err)
		}
	}()

	s.Log("Connecting service")
	if err := helper.ConnectToServiceWithTimeout(ctx, service, time.Minute); err != nil {
		s.Log("Connection failed: ", err)
	} else {
		s.Log("Connection succeeded")
	}

	s.Log("Waiting for disable goroutine")
	wg.Wait()
}
