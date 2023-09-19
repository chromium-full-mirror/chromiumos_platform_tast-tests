// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package croshealthd

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: fixture.CrosHealthdRebootDUT,
		Desc: "Reboots the DUT",
		Contacts: []string{
			"cros-tdm-tpe-eng@google.com", // Team mailing list
			"kerker@google.com",           // Fixture maintainer
			"yycheng@google.com",          // Fixture maintainer
		},
		SetUpTimeout: 5 * time.Minute,
		Impl:         rebootDUT{},
	})
}

// RebootDUT implements testing.FixtureImpl.
type rebootDUT struct {
}

// SetUp reboots the DUT.
func (rebootDUT) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	d := s.DUT()

	if err := d.Reboot(ctx); err != nil {
		return errors.Wrap(err, "failed to reboot DUT")
	}

	return nil
}

// Reset does nothing.
func (rebootDUT) Reset(ctx context.Context) error {
	return nil
}

// PreTest does nothing.
func (rebootDUT) PreTest(ctx context.Context, s *testing.FixtTestState) {}

// PostTest does nothing.
func (rebootDUT) PostTest(ctx context.Context, s *testing.FixtTestState) {}

// TearDown does nothing.
func (rebootDUT) TearDown(ctx context.Context, s *testing.FixtState) {}
