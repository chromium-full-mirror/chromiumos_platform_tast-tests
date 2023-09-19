// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package croshealthd

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// crosHealthdJobName is the name of the cros_healthd upstart job
const crosHealthdJobName = "cros_healthd"

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: "crosHealthdRunning",
		Desc: "The croshealthd daemon is available and running",
		Contacts: []string{
			"cros-tdm-tpe-eng@google.com", // Team mailing list
			"kerker@google.com",           // Fixture maintainer
			"menghuan@google.com",         // Fixture maintainer
		},
		SetUpTimeout:    30 * time.Second,
		ResetTimeout:    5 * time.Second,
		PreTestTimeout:  5 * time.Second,
		PostTestTimeout: 5 * time.Second,
		TearDownTimeout: 5 * time.Second,
		Impl:            newCrosHealthdFixture(),
		// Reboot the DUT before starting the healthd fixture to ensure that
		// sufficient resources are available by resetting to clean state. For
		// example, the cpu_stress and cpu_cache routine requires 628 MiB memory
		// before running, which may not be available depending on what tests were
		// ran before the healthd fixture.
		Parent: fixture.CrosHealthdRebootDUT,
	})
}

// crosHealthdFixture implements testing.FixtureImpl.
type crosHealthdFixture struct {
	// pid of cros_healthd, for check if it crashed or restarted within a single test.
	pid int
}

func newCrosHealthdFixture() testing.FixtureImpl {
	return &crosHealthdFixture{}
}

func (f *crosHealthdFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	if err := upstart.EnsureJobRunning(ctx, crosHealthdJobName); err != nil {
		return errors.Wrap(err, "failed to start cros_healthd")
	}
	return nil
}

func (f *crosHealthdFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	_, _, pid, err := upstart.JobStatus(ctx, crosHealthdJobName)
	if err != nil {
		s.Fatalf("Unable to get %s PID: %s", crosHealthdJobName, err)
	}

	f.pid = pid
}

func (f *crosHealthdFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	_, _, pid, err := upstart.JobStatus(ctx, crosHealthdJobName)
	if err != nil {
		s.Fatalf("Unable to get %s PID: %s", crosHealthdJobName, err)
	}

	// Make sure it has not crashed or restarted.
	if pid != f.pid {
		s.Fatalf("%s PID changed: want %v, got %v", crosHealthdJobName, f.pid, pid)
	}
}

func (f *crosHealthdFixture) Reset(ctx context.Context) error {
	if err := upstart.EnsureJobRunning(ctx, crosHealthdJobName); err != nil {
		return errors.Wrap(err, "failed to start cros_healthd")
	}
	return nil
}

func (f *crosHealthdFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	// cros_healthd should be running be default on all systems. Ensure it is
	// left running.
	if err := upstart.EnsureJobRunning(ctx, crosHealthdJobName); err != nil {
		s.Fatal(err, "failed to start cros_healthd")
	}
}
