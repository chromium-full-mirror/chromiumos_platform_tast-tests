// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fixture implements fixtures for Flex tests.
package fixture

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast/core/testing"
)

// Fixture names.
const (
	FlexWithServo = "flexWithServo"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            FlexWithServo,
		Desc:            "Fixture for ChromeOS Flex tests that use servo",
		Contacts:        []string{"chromeos-flex-eng+oncall@google.com", "josephsussman@google.com"},
		BugComponent:    "b:998633", // ChromeOS > Platform > Enablement > ChromeOS Flex
		Impl:            &withServoImpl{helper: &FixtData{}},
		Vars:            []string{"servo"},
		SetUpTimeout:    10 * time.Second,
		ResetTimeout:    10 * time.Second,
		PreTestTimeout:  10 * time.Second,
		PostTestTimeout: 10 * time.Second,
		TearDownTimeout: 10 * time.Second,
		Data:            []string{firmware.ConfigFile}, // Required to create a firmware.Helper
	})
}

// FixtData is the data returned by SetUp and passed to tests.
type FixtData struct {
	Helper *firmware.Helper
}

// withServoImpl implements testing.FixtureImpl.
type withServoImpl struct {
	helper *FixtData
}

// String identifies this fixture.
func (i *withServoImpl) String() string {
	return FlexWithServo
}

// SetUp is called once before the first test starts.
func (i *withServoImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	s.Log("Creating a new firmware Helper instance for fixture: ", i.String())
	if i.helper.Helper == nil {
		servoSpec := s.RequiredVar("servo")
		i.helper.Helper = firmware.NewHelper(s.DUT(), s.RPCHint(), s.DataPath(firmware.ConfigFile), servoSpec, "", "", "", "")
	}
	if err := i.helper.Helper.RequireServo(ctx); err != nil {
		s.Fatal("Test did not run. Failed to connect to servo: ", err)
	}
	connectTimeout, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()
	if err := i.helper.Helper.WaitConnect(connectTimeout); err != nil {
		s.Fatal("Test did not run. Failed to connect to DUT: ", err)
	}
	return i.helper
}

// PreTest runs before every test.
func (i *withServoImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
	// Write an echo to servod, so the test name appears in the logs.
	if _, err := i.helper.Helper.Servo.Echo(ctx, fmt.Sprintf("Test start: %s", s.TestName())); err != nil {
		s.Fatal("Test did not run. Servo echo failed: ", err)
	}
}

// PostTest runs after every test.
func (i *withServoImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
}

// Reset runs after all but the last test to roll back changes made to the environment.
func (i *withServoImpl) Reset(ctx context.Context) error {
	i.helper.Helper.CloseServo(ctx)
	return nil
}

// TearDown is called once just after the last test completes, unless SetUp fails.
func (i *withServoImpl) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := i.helper.Helper.Close(ctx); err != nil {
		s.Fatal("Failed to close helper: ", err)
	}
	i.helper.Helper = nil
}
