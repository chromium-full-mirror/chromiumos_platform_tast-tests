// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fixture contains the base fixtures for all ML benchmarks.
package fixture

import (
	"context"
	"os"

	"go.chromium.org/tast-tests/cros/local/mlbenchmark"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// MLBenchmark is the base fixture for the all MLBenchmark tests.
	MLBenchmark = "mlBenchmark"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: MLBenchmark,
		Desc: "The base fixture for all ML Benchmarks",
		Contacts: []string{
			"chrome-knowledge-eng@google.com",
			"jmpollock@google.com",
		},
		Impl:   baseSetupFixture(),
		Parent: setup.PowerNoUIWiFi,
	})
}

type baseSetupFixtureImpl struct{}

func baseSetupFixture() testing.FixtureImpl {
	return &baseSetupFixtureImpl{}
}

func (f *baseSetupFixtureImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	return nil
}

func (f *baseSetupFixtureImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
	// Clear out any old / stale data.
	if err := resetDataDirectory(); err != nil {
		s.Fatal(err, "failed to reset data directory %s", mlbenchmark.DataDirectory)
	}
}

func (f *baseSetupFixtureImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {}

func (f *baseSetupFixtureImpl) Reset(ctx context.Context) error {
	return nil
}

func (f *baseSetupFixtureImpl) TearDown(ctx context.Context, s *testing.FixtState) {
	// Return the system to a 'clean' state.
	if err := removeDataDirectory(); err != nil {
		s.Fatal(err, "failed to reset data directory %s", mlbenchmark.DataDirectory)
	}
}

func removeDataDirectory() error {
	if _, err := os.Stat(mlbenchmark.DataDirectory); !os.IsNotExist(err) {
		if err := os.RemoveAll(mlbenchmark.DataDirectory); err != nil {
			return errors.Wrapf(err, "failed to clear data directory %s", mlbenchmark.DataDirectory)
		}
	}
	return nil
}

func resetDataDirectory() error {
	removeDataDirectory()
	if err := os.MkdirAll(mlbenchmark.DataDirectory, 0755); err != nil {
		return errors.Wrapf(err, "failed to create data directory %s ", mlbenchmark.DataDirectory)
	}
	return nil
}
