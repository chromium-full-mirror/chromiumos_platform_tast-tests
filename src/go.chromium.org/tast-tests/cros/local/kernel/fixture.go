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
	hrtimerPath = "/proc/sys/kernel/timer_highres"
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
}

type highResTimerOffFixture struct{}

func (i *highResTimerOffFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	if err := os.WriteFile(hrtimerPath, []byte("0"), 0); err != nil {
		s.Fatal("Failed to turn off hrtimer: ", err)
	}
	return nil
}

func (i *highResTimerOffFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := os.WriteFile(hrtimerPath, []byte("1"), 1); err != nil {
		s.Fatal("Failed to reset hrtimer: ", err)
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
