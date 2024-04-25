// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package starfish

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/cellular"
	"go.chromium.org/tast-tests/cros/local/starfish"
	"go.chromium.org/tast/core/testing"
)

// The starfish test fixture ensures that the correct SIM slot is configured for the test.

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            "starfish",
		Desc:            "Allows preconfiguration of the Starfish module before running a test suite",
		Contacts:        []string{"chromeos-cellular-team@google.com", "andrewlassalle@google.com"},
		BugComponent:    "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		SetUpTimeout:    1 * time.Minute,
		ResetTimeout:    1 * time.Second,
		PreTestTimeout:  1 * time.Second,
		PostTestTimeout: 1 * time.Minute,
		TearDownTimeout: 1 * time.Second,
		Impl:            &starfishFixture{},
	})
}

// starfishFixture implements testing.FixtureImpl.
type starfishFixture struct {
	sf *starfish.Starfish
}

// FixtData holds information made available to tests that specify this fixture.
type FixtData struct {
}

func (f *starfishFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	var dutInfo *cellular.DUTInfo
	if dutConfig, err := s.ChromeOSDUTLabConfig(""); err == nil {
		dutInfo = cellular.NewDUTInfoFromConfig(dutConfig)
	} else if dutInfo, err = cellular.NewDUTInfoFromStringArgs(ctx, s.Var, "autotest_host_info_labels"); err != nil {
		s.Fatal("Failed to get SIM info labels: ", err)
	}

	sfish, _, _, err := starfish.NewStarfish(ctx, dutInfo)
	if err != nil {
		s.Fatal("Failed to setup starfish module on supported setup: ", err)
	}
	f.sf = sfish
	return &FixtData{}
}

func (f *starfishFixture) Reset(ctx context.Context) error {
	return nil
}

func (f *starfishFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *starfishFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *starfishFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if f.sf == nil {
		return
	}
	if err := f.sf.Teardown(ctx); err != nil {
		s.Fatalf("Failed to teardown starfish: %s", err)
	}
}
