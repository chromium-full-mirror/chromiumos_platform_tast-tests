// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/power/suspend"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           ShillStressSuspendResume,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		Desc:           "Stress test suspend resume, verify modem is in right state after each resume",
		Contacts:       []string{"chromeos-cellular-team@google.com", "rmao@google.com"},
		BugComponent:   "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:           []string{"group:cellular", "cellular_unstable", "cellular_stress"},
		Fixture:        "cellular",
		Timeout:        60 * time.Minute,
		SoftwareDeps:   []string{"chrome"},
	})
}

func ShillStressSuspendResume(ctx context.Context, s *testing.State) {
	helper := s.FixtValue().(*cellular.FixtData).Helper

	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to start Chrome (precondition): ", err)
	}
	defer cr.Close(ctx)

	const totalAttempts = 100
	const iterations = 5
	const suspendTimeSec = 10
	stressConnection := func(ctx context.Context) error {
		for i := 1; i <= iterations; i++ {
			s.Logf("Test iteration: #%d", i)

			if _, err = cr.TestAPIConn(ctx); err != nil {
				return errors.Wrap(err, "failed to establish the Test API connection")
			}

			service, err := helper.FindServiceForDevice(ctx)
			if err != nil {
				return errors.Wrap(err, "Unable to find Cellular Service for Device")
			}

			if err := service.WaitForConnectedOrError(ctx); err != nil {
				return errors.Wrapf(err, "Connect failed on attempt %d", i)
			}

			if _, err := suspend.ForDuration(ctx, suspendTimeSec*time.Second); err != nil {
				return errors.Wrap(err, "failed to perform system suspend")
			}

			if err = cr.Reconnect(ctx); err != nil {
				return errors.Wrap(err, "failed to reconnect to Chrome")
			}
		}
		return nil
	}

	for i := 1; i <= totalAttempts/iterations; i++ {
		s.Logf("Test round: #%d", i)
		if err := helper.RunTestOnCellularInterface(ctx, stressConnection); err != nil {
			s.Fatal("Failed to run test on cellular interface: ", err)
		}
		// GoBigSleepLint: A short sleep before we disable ethernet and wifi again.
		testing.Sleep(ctx, 10*time.Second)
	}
}
