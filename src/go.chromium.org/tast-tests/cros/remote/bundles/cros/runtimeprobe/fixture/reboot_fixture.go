// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runtimeprobe

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/runtimeprobe/utils"
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
	d := s.DUT()

	s.Log("Rebooting the DUT")
	if err := d.Reboot(ctx); err != nil {
		s.Fatal("Failed to reboot DUT: ", err)
	}

	if err := utils.WaitServiceState(ctx, d, "system-services", "start/running"); err != nil {
		s.Fatal("Service system-services timed out: ", err)
	}

	if err := utils.WaitServiceState(ctx, d, "hardware_verifier", "stop/waiting"); err != nil {
		s.Fatal("Service hardware_verifier timed out: ", err)
	}

	s.Log("hardware_verifier finished running")
	return nil
}

func (f *rebootForProbeFunctionFixture) TearDown(ctx context.Context, s *testing.FixtState)     {}
func (f *rebootForProbeFunctionFixture) Reset(ctx context.Context) error                        { return nil }
func (f *rebootForProbeFunctionFixture) PreTest(ctx context.Context, s *testing.FixtTestState)  {}
func (f *rebootForProbeFunctionFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}
