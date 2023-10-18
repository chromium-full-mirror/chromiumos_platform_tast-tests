// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package kernel

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome"

	"go.chromium.org/tast/core/testing"
)

const (
	hrtimerPath         = "/proc/sys/kernel/timer_highres"
	schedAggressivePath = "/proc/sys/kernel/sched_aggressive_next_balance"
	schedMinLoadPath    = "/proc/sys/kernel/sched_min_load_balance_interval"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            fixture.HighResTimerOff,
		Desc:            "Fixture for turning off hrtimer",
		Contacts:        []string{"cros-sw-perf@google.com", "hsinyi@google.com"},
		Impl:            &highResTimerOffFixture{},
		SetUpTimeout:    chrome.ManagedUserLoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		PostTestTimeout: 15 * time.Second,
	})
	testing.AddFixture(&testing.Fixture{
		Name:            fixture.HighResTimerOffEnrolled,
		Desc:            "Fixture for enrollment and turning off hrtimer",
		Contacts:        []string{"cros-sw-perf@google.com", "hsinyi@google.com"},
		Impl:            &highResTimerOffFixture{},
		Parent:          fixture.Enrolled, // Provides enrollment.
		SetUpTimeout:    chrome.ManagedUserLoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		PostTestTimeout: 15 * time.Second,
	})
	testing.AddFixture(&testing.Fixture{
		Name:            fixture.HighResTimerOffGpuWatchHangs,
		Desc:            "Fixture for gpuWatchHangs and turning off hrtimer",
		Contacts:        []string{"cros-sw-perf@google.com", "hsinyi@google.com"},
		Impl:            &highResTimerOffFixture{},
		Parent:          "gpuWatchHangs", // Provides enrollment.
		SetUpTimeout:    chrome.ManagedUserLoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		PostTestTimeout: 15 * time.Second,
	})
}

type highResTimerOffFixture struct{}

func (i *highResTimerOffFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	// The default value of timer_highres, sched_aggressive_next_balance, and
	// sched_min_load_balance_interval are 1, 1, 1. Set them to 0, 0, 16
	// respectively to turn off highres timer.
	if err := os.WriteFile(hrtimerPath, []byte("0"), 0644); err != nil {
		s.Fatal("Failed to turn off hrtimer: ", err)
	}
	if err := os.WriteFile(schedAggressivePath, []byte("0"), 0644); err != nil {
		s.Fatal("Failed to set sched_aggressive_next_balance: ", err)
	}
	if err := os.WriteFile(schedMinLoadPath, []byte("16"), 0644); err != nil {
		s.Fatal("Failed to set sched_min_load_balance_interval: ", err)
	}
	return nil
}

func (i *highResTimerOffFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	// Restore back the default value.
	if err := os.WriteFile(hrtimerPath, []byte("1"), 0644); err != nil {
		s.Fatal("Failed to reset hrtimer: ", err)
	}
	if err := os.WriteFile(schedAggressivePath, []byte("1"), 0644); err != nil {
		s.Fatal("Failed to reset sched_aggressive_next_balance: ", err)
	}
	if err := os.WriteFile(schedMinLoadPath, []byte("1"), 0644); err != nil {
		s.Fatal("Failed to reset sched_min_load_balance_interval: ", err)
	}
}

func (i *highResTimerOffFixture) Reset(ctx context.Context) error {
	return nil
}

func (i *highResTimerOffFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	// No-op.
}

func (i *highResTimerOffFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	// No-op.
}
