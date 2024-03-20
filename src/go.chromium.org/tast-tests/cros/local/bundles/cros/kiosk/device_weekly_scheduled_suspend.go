// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package kiosk

import (
	"context"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/kioskmode"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/syslog"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Common terminology used in this file:
//   - 'suspend' is the operation that puts the device to sleep,
//   - 'resume' is the opposite operation that wakes up the device.
//   - 'sleep' is the state after a suspend operation, and before a resume.
//
// The policy name `DeviceWeeklyScheduledSuspend` is a slight deviation, as it
// schedules sleep intervals.
const (
	policyPropagationDelay = 3 * time.Second
	intervalBetweenSleeps  = 5 * time.Second
	sleepShortDuration     = 3 * time.Second
	sleepLongDuration      = 7 * time.Second
	sleepPollTimeout       = policyPropagationDelay + intervalBetweenSleeps + sleepLongDuration

	// sleepTolerance is how much a duration reported by SuspendDone can deviate
	// from the scheduled duration. In practice it's usually off by ~1 second.
	sleepTolerance = 2 * time.Second
)

var (
	// suspendDoneRegexp matches SuspendDone signals as logged by PowerManagerClient.
	suspendDoneRegexp      = regexp.MustCompile(`Got SuspendDone signal:.* duration=(\d+) sec`)
	errSuspendDoneNotFound = errors.New("not a SuspendDone log entry")
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DeviceWeeklyScheduledSuspend,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that kiosk device sleeps on schedule with policy DeviceWeeklyScheduledSuspend",
		Contacts:     []string{"chromeos-kiosk-eng+TAST@google.com"},
		BugComponent: "b:892153", // ChromeOS > Software > Commercial (Enterprise) > Kiosk
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
		},
		SoftwareDeps: []string{"reboot", "chrome", "vpd"},
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		Fixture:      fixture.FakeDMSEnrolled,
		Timeout:      sleepPollTimeout*2 + kioskmode.SetupDuration + 2*kioskmode.LaunchDuration + kioskmode.CleanupDuration,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceWeeklyScheduledSuspend{}, pci.VerifiedFunctionalityOS),
		},
	})
}

func DeviceWeeklyScheduledSuspend(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, kioskmode.CleanupDuration)
	defer cancel()

	signinTestExtensionManifestKey := s.RequiredVar("ui.signinProfileTestExtensionManifestKey")
	kiosk, cr, err := kioskmode.New(
		ctx,
		fdms,
		signinTestExtensionManifestKey,
		kioskmode.AutoLaunch(kioskmode.KioskAppAccountID),
		// Enable device event log in order to observe powerd events.
		kioskmode.ExtraChromeOptions(chrome.ExtraArgs("--vmodule=device_event_log*=1")),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome in Kiosk mode: ", err)
	}
	defer func(ctx context.Context) {
		if err := kiosk.Close(ctx); err != nil {
			s.Error("Failed to close Kiosk: ", err)
		}
	}(cleanupCtx)

	if err := kiosk.WaitLaunchLogs(ctx); err != nil {
		s.Fatal("Failed to launch Kiosk: ", err)
	}

	// Start a reader for Chrome logs (with device events).
	reader, err := syslog.NewChromeReader(ctx, "/var/log/chrome/chrome")
	defer reader.Close()

	// Schedule sleep intervals shortly after Kiosk has launched.
	pb := kiosk.PolicyBlob()
	policy := buildPolicyDeviceWeeklyScheduledSuspend()
	if err := pb.AddPolicy(policy); err != nil {
		s.Fatal("Failed to add policy: ", err)
	}
	if err := policyutil.ServeBlobAndRefresh(ctx, fdms, cr, pb); err != nil {
		s.Fatal("Failed to serve and refresh policies: ", err)
	}

	// Verify sleeps happened on schedule.
	if err := verifyNextSuspendDoneDuration(ctx, reader, sleepShortDuration); err != nil {
		s.Fatal("Failed suspend and resume #1: ", err)
	}
	if err := verifyNextSuspendDoneDuration(ctx, reader, sleepLongDuration); err != nil {
		s.Fatal("Failed suspend and resume #2: ", err)
	}
}

func buildPolicyDeviceWeeklyScheduledSuspend() policy.Policy {
	start1 := time.Now().Add(policyPropagationDelay)
	end1 := start1.Add(sleepShortDuration)
	interval1 := deviceWeeklyScheduledSuspendValue(start1, end1)

	start2 := end1.Add(intervalBetweenSleeps)
	end2 := start2.Add(sleepLongDuration)
	interval2 := deviceWeeklyScheduledSuspendValue(start2, end2)

	schedule := []*policy.DeviceWeeklyScheduledSuspendValue{interval1, interval2}
	return &policy.DeviceWeeklyScheduledSuspend{Val: schedule}
}

func deviceWeeklyScheduledSuspendValue(start, end time.Time) *policy.DeviceWeeklyScheduledSuspendValue {
	return &policy.DeviceWeeklyScheduledSuspendValue{
		Start: &policy.DeviceWeeklyScheduledSuspendValueStart{DayOfWeek: dayOfWeek(start), Time: intradayMs(start)},
		End:   &policy.DeviceWeeklyScheduledSuspendValueEnd{DayOfWeek: dayOfWeek(end), Time: intradayMs(end)},
	}
}

func dayOfWeek(t time.Time) string {
	return strings.ToUpper(t.Weekday().String())
}

func intradayMs(t time.Time) int {
	year, month, day := t.Date()
	midnight := time.Date(year, month, day, 0, 0, 0, 0, time.Local)
	intradayMilliseconds := t.Sub(midnight).Milliseconds()
	return int(intradayMilliseconds)
}

func verifyNextSuspendDoneDuration(ctx context.Context, reader *syslog.ChromeReader, wantDuration time.Duration) error {
	testing.ContextLog(ctx, "Waiting for SuspendDone signal with duration ", wantDuration)

	var duration time.Duration
	err := testing.Poll(ctx, func(ctx context.Context) error {
		for {
			entry, err := reader.Read()
			if errors.Is(err, io.EOF) {
				return err
			}
			if err != nil {
				return testing.PollBreak(errors.Wrap(err, "failed to find SuspendDone signal"))
			}
			seconds, parseErr := parseSuspendDoneDuration(entry)
			if errors.Is(parseErr, errSuspendDoneNotFound) {
				continue
			}
			if parseErr != nil {
				return testing.PollBreak(errors.Wrap(parseErr, "error parsing SuspendDone duration"))
			}
			duration = time.Duration(seconds) * time.Second
			return nil
		}
	}, &testing.PollOptions{Timeout: sleepPollTimeout})
	if err != nil {
		return errors.Wrap(err, "error waiting for SuspendDone signal")
	}
	testing.ContextLog(ctx, "Detected sleep duration: ", duration)
	if (wantDuration - duration).Abs() > sleepTolerance {
		return errors.Errorf("sleep duration = %v, want %v", duration, wantDuration)
	}
	return nil
}

func parseSuspendDoneDuration(entry *syslog.ChromeEntry) (int, error) {
	if m := suspendDoneRegexp.FindStringSubmatch(entry.Content); m != nil {
		return strconv.Atoi(m[1])
	}
	return 0, errSuspendDoneNotFound
}
