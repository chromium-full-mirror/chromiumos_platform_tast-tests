// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package ehide contains code to hide the Ethernet.
package ehide

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast-tests/cros/common/network/ehideconst"
	"go.chromium.org/tast-tests/cros/common/testexec"
)

// The timeout for setup and teardown. Set it a bit longer than
// waitForEhideStateTimeout.
const ehideTimeout = 30 * time.Second

// In ehide we wait for 10 seconds for the setup of IPv4 and IPv6 addresses,
// each. Here we wait a maximum of 25 seconds, which is slightly longer than
// the sum of the maximum possible waiting time for the appearance of IPv4 and
// IPv6 addresses.
const waitForEhideStateTimeout = 25 * time.Second
const waitForEhideStateInterval = 1 * time.Second
const postTestTimeout = 5 * time.Second

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: "ehide",
		Desc: "A fixture that hides the Ethernet",
		Contacts: []string{
			"cros-networking@google.com",
			"chenzikai@google.com",
		},
		BugComponent:    "b:1493959", // ChromeOS > Platform > baseOS > Networking > Continuous Maintenance
		SetUpTimeout:    ehideTimeout,
		TearDownTimeout: ehideTimeout,
		PostTestTimeout: postTestTimeout,
		Impl:            &ehideFixture{},
	})
}

// ehideFixture implements testing.FixtureImpl.
type ehideFixture struct {
}

func (f *ehideFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	d := s.DUT()
	if err := d.Conn().CommandContext(ctx, ehideconst.EhidePath, "start").Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to start ehide: ", err)
	}

	if err := waitForEhideState(ctx, d, ehideconst.EhideStateOn); err != nil {
		s.Fatal("Failed to wait for ehide turning on: ", err)
	}
	return nil
}

func (f *ehideFixture) Reset(ctx context.Context) error {
	return nil
}

func (f *ehideFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {}

func (f *ehideFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	if ehideState, err := getState(ctx, s.DUT()); err != nil {
		s.Fatal("Failed to get ehide state: ", err)
	} else if ehideState != ehideconst.EhideStateOn {
		s.Fatalf("Got current state %s, want %s", ehideState, ehideconst.EhideStateOn)
	}
}

func (f *ehideFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	d := s.DUT()
	if err := d.Conn().CommandContext(ctx, ehideconst.EhidePath, "stop").Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to stop ehide: ", err)
	}

	if err := waitForEhideState(ctx, d, ehideconst.EhideStateOff); err != nil {
		s.Fatal("Failed to wait for ehide turing off: ", err)
	}
}

func getState(ctx context.Context, dut *dut.DUT) (string, error) {
	output, err := dut.Conn().CommandContext(ctx, ehideconst.EhidePath, "state").Output(testexec.DumpLogOnError)
	if err != nil {
		return "", err
	}
	state := strings.TrimSpace(string(output))
	return state, nil
}

func waitForEhideState(ctx context.Context, dut *dut.DUT, state string) error {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if e := dut.Connect(ctx); e != nil {
			return e
		}
		if s, e := getState(ctx, dut); e != nil {
			return e
		} else if s != state {
			return errors.Errorf("got current state %s, want %s", s, state)
		}
		return nil
	}, &testing.PollOptions{Timeout: waitForEhideStateTimeout, Interval: waitForEhideStateInterval}); err != nil {
		return errors.Errorf("failed to wait for ehide: %s", err)
	}
	return nil
}
