// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runtimeprobe

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: fixture.RebootForProbeFunction,
		Desc: "Reboots the DUT before ProbeFunction test cases",
		Contacts: []string{
			"chromeos-runtime-probe@google.com",
			"clarkchung@google.com",
			"allenshihmc@google.com",
		},
		BugComponent: "b:606088",
		Impl:         &rebootForProbeFunctionFixture{},
		SetUpTimeout: 5 * time.Minute,
	})
}

type rebootForProbeFunctionFixture struct{}

func (f *rebootForProbeFunctionFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	const (
		pollInterval = 3 * time.Second
		pollTimeout  = 2 * time.Minute
	)

	d := s.DUT()

	s.Log("Rebooting the DUT")
	if err := d.Reboot(ctx); err != nil {
		return errors.Wrap(err, "failed to reboot DUT")
	}
	s.Log("DUT rebooted successfully. Waiting for hardware_verifier to finish running")

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		output, err := d.Conn().CommandContext(ctx, "initctl", "status", "hardware_verifier").Output()
		if err != nil {
			return err
		}
		if !strings.Contains(string(output), "stop/waiting") {
			return errors.New("hardware_verifier is not stop/waiting state")
		}
		return nil
	}, &testing.PollOptions{Interval: pollInterval, Timeout: pollTimeout}); err != nil {
		s.Fatal("Failed to wait for hardware_verifier to be stop/waiting: ", err)
	}

	s.Log("hardware_verifier finished running")
	return nil
}

func (f *rebootForProbeFunctionFixture) TearDown(ctx context.Context, s *testing.FixtState)     {}
func (f *rebootForProbeFunctionFixture) Reset(ctx context.Context) error                        { return nil }
func (f *rebootForProbeFunctionFixture) PreTest(ctx context.Context, s *testing.FixtTestState)  {}
func (f *rebootForProbeFunctionFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}
