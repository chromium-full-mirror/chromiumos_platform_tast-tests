// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    KBPress,
		Desc:    "Verify kbpress console command mirrors strobe signals and expires correctly",
		Timeout: 5 * time.Minute,
		Contacts: []string{
			"cros-hwsec@google.com",
			"wisniewskib@google.com",
		},
		BugComponent: "b:715469",
		// TODO(b/513305476): enable after crrev.com/i/9347641
		// Attr:         []string{"group:gsc", "gsc_image_ti50", "gsc_nightly"},
		Fixture: fixture.GSCOpenCCD,
	})
}

func KBPress(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s)
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	s.Log("Starting GSC with Clamshell straps and CCD enabled")
	b.ResetWithStraps(ctx, ti50.FfClamshell, ti50.CCDModeOn)
	if err := i.WaitUntilBooted(ctx); err != nil {
		s.Fatal("Ti50 failed to boot: ", err)
	}

	strobePin := ti50.GpioTi50EcKso2Inv
	outPin := ti50.GpioTi50EcKsi2

	// Initial state: strobe inactive (high)
	b.GpioSet(ctx, strobePin, true)

	s.Log("Starting GPIO monitor on ", outPin)
	mon := b.GpioMonitorStart(ctx, outPin)

	s.Log("Running 'kbpress 0 1000'")
	if _, err := i.Command(ctx, "kbpress 0 1000"); err != nil {
		s.Fatal("Failed to run kbpress command: ", err)
	}

	// Simulate EC column strobe assertion during active window (drive low)
	s.Log("Asserting EC strobe (low) on ", strobePin)
	b.GpioSet(ctx, strobePin, false)

	testing.Sleep(ctx, 50*time.Millisecond) // GoBigSleepLint: Simulating EC strobe duration

	s.Log("Releasing EC strobe (high) on ", strobePin)
	b.GpioSet(ctx, strobePin, true)

	testing.Sleep(ctx, 50*time.Millisecond) // GoBigSleepLint: Allowing strobe release to propagate

	events := b.GpioMonitorFinish(ctx, mon)
	s.Log("GPIO Events during active window: ", events.Sorted)

	fall := events.FindFirst(outPin, utils.GpioEdgeFalling)
	if fall == nil {
		s.Fatal("Failed to detect output falling edge (assertion) upon strobe")
	}

	rise := events.FindFirstAfter(*fall, outPin)
	if rise == nil || rise.Edge != utils.GpioEdgeRising {
		s.Fatal("Failed to detect output rising edge (deassertion)")
	}

	s.Log("Confirmed key was asserted and released successfully upon strobe")

	// Wait past the 1000ms expiration window
	s.Log("Waiting for 1000ms simulation timer to expire")
	testing.Sleep(ctx, 1500*time.Millisecond) // GoBigSleepLint: Waiting for 1000ms simulation timer to expire

	s.Log("Verifying strobe is ignored after timer expiration")
	monExpired := b.GpioMonitorStart(ctx, outPin)

	b.GpioSet(ctx, strobePin, false)
	testing.Sleep(ctx, 50*time.Millisecond) // GoBigSleepLint: Simulating expired strobe duration
	b.GpioSet(ctx, strobePin, true)
	testing.Sleep(ctx, 50*time.Millisecond) // GoBigSleepLint: Allowing expired strobe release to propagate

	eventsExpired := b.GpioMonitorFinish(ctx, monExpired)
	s.Log("GPIO Events after expiration: ", eventsExpired.Sorted)

	if len(eventsExpired.Sorted) > 0 {
		s.Errorf("Output %s unexpectedly toggled upon strobe after timer expired: %v", outPin, eventsExpired.Sorted)
	}
}
