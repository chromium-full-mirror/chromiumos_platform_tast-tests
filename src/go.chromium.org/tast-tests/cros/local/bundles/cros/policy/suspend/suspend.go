// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package suspend provides helpers for testing suspend/resume policies.
package suspend

import (
	"context"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/syslog"
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
	// SleepTolerance is how much a detected sleep duration can deviate from the
	// scheduled duration. It is off by ~1s on fast devices, but could be off by 4s
	// or more on slow devices. Larger values reduce flakiness, but prolong the test.
	SleepTolerance = 5 * time.Second

	SleepShortDuration     = SleepTolerance + time.Second
	SleepLongDuration      = SleepShortDuration * 2
	IntervalBetweenSleeps  = 30 * time.Second
	PolicyPropagationDelay = 15 * time.Second
	SleepPollTimeout       = PolicyPropagationDelay + IntervalBetweenSleeps + SleepLongDuration
)

var (
	// suspendDoneRegexp matches SuspendDone signals as logged by PowerManagerClient.
	suspendDoneRegexp      = regexp.MustCompile(`Got SuspendDone signal:.* duration=(\d+) sec`)
	errSuspendDoneNotFound = errors.New("not a SuspendDone log entry")
)

// BuildDeviceWeeklyScheduledSuspend builds a DeviceWeeklyScheduledSuspend policy
// with a short and a long sleep interval starting shortly after the current time.
func BuildDeviceWeeklyScheduledSuspend() policy.Policy {
	start1 := time.Now().Add(PolicyPropagationDelay)
	end1 := start1.Add(SleepShortDuration)
	interval1 := deviceWeeklyScheduledSuspendValue(start1, end1)

	start2 := end1.Add(IntervalBetweenSleeps)
	end2 := start2.Add(SleepLongDuration)
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

// VerifyNextSuspendDoneDuration polls the syslog reader for a SuspendDone signal
// that roughly matches wantDuration.
func VerifyNextSuspendDoneDuration(ctx context.Context, reader *syslog.ChromeReader, wantDuration time.Duration) error {
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
	}, &testing.PollOptions{Timeout: SleepPollTimeout})
	if err != nil {
		return errors.Wrap(err, "error waiting for SuspendDone signal")
	}
	testing.ContextLog(ctx, "Detected sleep duration: ", duration)
	if (wantDuration - duration).Abs() > SleepTolerance {
		return errors.Errorf("sleep duration = %v, want %v", duration, wantDuration)
	}
	return nil
}

// VerifyNoSuspend waits for the expected schedule duration and ensures no SuspendDone signal is logged.
func VerifyNoSuspend(ctx context.Context, reader *syslog.ChromeReader) error {
	waitDuration := PolicyPropagationDelay + SleepShortDuration + IntervalBetweenSleeps + SleepLongDuration + 5*time.Second
	testing.ContextLogf(ctx, "Waiting %v to ensure no suspend happens", waitDuration)

	// GoBigSleepLint: Wait for the duration of the scheduled suspends to ensure none occur.
	if err := testing.Sleep(ctx, waitDuration); err != nil {
		return errors.Wrap(err, "failed waiting")
	}

	for {
		entry, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return errors.Wrap(err, "error reading logs")
		}
		if _, err := parseSuspendDoneDuration(entry); err == nil {
			return errors.New("device suspended when it should not have")
		}
	}
	return nil
}

func parseSuspendDoneDuration(entry *syslog.ChromeEntry) (int, error) {
	if m := suspendDoneRegexp.FindStringSubmatch(entry.Content); m != nil {
		return strconv.Atoi(m[1])
	}
	return 0, errSuspendDoneNotFound
}

// VerifyScheduledSuspend adds the DeviceWeeklyScheduledSuspend policy to the blob,
// serves and refreshes policies, and verifies whether the device suspends on schedule.
func VerifyScheduledSuspend(ctx context.Context, fdms *fakedms.FakeDMS, cr *chrome.Chrome, pb *policy.Blob, shouldSuspend, isLoginScreen bool) error {
	reader, err := syslog.NewChromeReader(ctx, "/var/log/chrome/chrome")
	if err != nil {
		return errors.Wrap(err, "failed to start log reader")
	}
	defer reader.Close()

	pol := BuildDeviceWeeklyScheduledSuspend()
	if err := pb.AddPolicy(pol); err != nil {
		return errors.Wrap(err, "failed to add scheduled suspend policy")
	}

	return ServeAndVerify(ctx, fdms, cr, pb, shouldSuspend, isLoginScreen, reader)
}

// ServeAndVerify serves and refreshes the policy blob and verifies that the
// device either suspends for the expected intervals or does not suspend at all.
func ServeAndVerify(ctx context.Context, fdms *fakedms.FakeDMS, cr *chrome.Chrome, pb *policy.Blob, shouldSuspend, isLoginScreen bool, reader *syslog.ChromeReader) error {
	if isLoginScreen {
		if err := policyutil.ServeBlobAndRefreshOnLoginScreen(ctx, fdms, cr, pb); err != nil {
			return errors.Wrap(err, "failed to serve and refresh policies on login screen")
		}
	} else {
		if err := policyutil.ServeBlobAndRefresh(ctx, fdms, cr, pb); err != nil {
			return errors.Wrap(err, "failed to serve and refresh policies")
		}
	}

	if shouldSuspend {
		if err := VerifyNextSuspendDoneDuration(ctx, reader, SleepShortDuration); err != nil {
			return errors.Wrap(err, "failed suspend and resume #1")
		}
		if err := VerifyNextSuspendDoneDuration(ctx, reader, SleepLongDuration); err != nil {
			return errors.Wrap(err, "failed suspend and resume #2")
		}
	} else {
		if err := VerifyNoSuspend(ctx, reader); err != nil {
			return errors.Wrap(err, "failed verifying no suspend")
		}
	}
	return nil
}
