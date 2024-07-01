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

// A relatively large timeout for fixture stability.
const ehideTimeout = 1 * time.Minute

// The timeout to wait for connection at the beginning.
const waitConnectTimeout = 30 * time.Second

// Set the connection timeout to a relatively long time (15 seconds) so that we
// can make sure that the connection can be established within the time limit
// if SSH is ready.
const connTimeout = 15 * time.Second

// The getState() function should return immediately. If it has not returned in
// 15 seconds, we can infer that the SSH connection has hanged due to the ehide
// startup or shutdown.
const getStateTimeout = 15 * time.Second
const waitForEhideStateInterval = 1 * time.Second
const postTestTimeout = 15 * time.Second

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
		Impl:            &ehideFixture{alreadyStarted: false},
	})
}

// ehideFixture implements testing.FixtureImpl.
type ehideFixture struct {
	alreadyStarted bool // whether ehide has already started before the fixture setup
}

func (f *ehideFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	d := s.DUT()

	// Since the SSH connection is not guaranteed at the remote fixture setup
	// (b/239013478), make sure the DUT is connected at the beginning.
	if err := d.Health(ctx); err != nil {
		s.Log("Failed DUT connection check at the beginning: ", err)

		// Try to reconnect to the DUT.
		waitConnectCtx, waitConnectCancel := context.WithTimeout(ctx, waitConnectTimeout)
		defer waitConnectCancel()
		if err := d.WaitConnect(waitConnectCtx); err != nil {
			s.Fatal("Failed to wait for DUT connection at the beginning: ", err)
		}
	}

	// Check whether ehide has already started.
	if ehideState, err := getState(ctx, d); err != nil {
		s.Fatal("Failed to get ehide state: ", err)
	} else {
		f.alreadyStarted = ehideState == ehideconst.EhideStateOn
	}

	// If ehide has already started, don't start it again.
	if f.alreadyStarted {
		s.Log("Ehide has already started")
		return nil
	}

	startErr := d.Conn().CommandContext(ctx, ehideconst.EhidePath, "start").Run(testexec.DumpLogOnError)
	if startErr != nil && strings.Contains(startErr.Error(), "Process exited with status") {
		// "Process exited with status" indicates that ehide has exited due to
		// an unexpected error. Report it and return here.
		s.Fatal("Failed to start ehide: ", startErr)
	}
	// Otherwise, either there is no error, or the error comes from the SSH
	// connection closed by ehide. This often happens so don't log anything
	// here. Only report the error if the ehide state verification fails later.

	if err := waitForEhideState(ctx, d, ehideconst.EhideStateOn); err != nil {
		if startErr != nil {
			s.Error("Failed to start ehide: ", startErr)
		}
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

	// If ehide has already started at the beginning, it could be either run
	// intentionally, or left over from the previous session. Either way, don't
	// stop it.
	if f.alreadyStarted {
		s.Log("Won't stop ehide since it was running at the beginning")
		return
	}

	stopErr := d.Conn().CommandContext(ctx, ehideconst.EhidePath, "stop").Run(testexec.DumpLogOnError)
	if stopErr != nil && strings.Contains(stopErr.Error(), "Process exited with status") {
		// "Process exited with status" indicates that ehide has exited due to
		// an unexpected error. Report it and return here.
		s.Fatal("Failed to stop ehide: ", stopErr)
	}
	// Otherwise, either there is no error, or the error comes from the SSH
	// connection closed by ehide. This often happens so don't log anything
	// here. Only report the error if the ehide state verification fails later.

	if err := waitForEhideState(ctx, d, ehideconst.EhideStateOff); err != nil {
		if stopErr != nil {
			s.Error("Failed to stop ehide: ", stopErr)
		}
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
		connCtx, connCancel := context.WithTimeout(ctx, connTimeout)
		defer connCancel()
		if err := dut.Connect(connCtx); err != nil {
			return err
		}
		getStateCtx, getStateCancel := context.WithTimeout(ctx, getStateTimeout)
		defer getStateCancel()
		if s, err := getState(getStateCtx, dut); err != nil {
			return err
		} else if s != state {
			return errors.Errorf("got current state %s, want %s", s, state)
		}
		return nil
	}, &testing.PollOptions{
		// We don't specify the polling timeout so that the timeout only happens
		// when the context exceeds the deadline.
		Interval: waitForEhideStateInterval,
	}); err != nil {
		return errors.Errorf("failed to wait for ehide: %s", err)
	}
	return nil
}
