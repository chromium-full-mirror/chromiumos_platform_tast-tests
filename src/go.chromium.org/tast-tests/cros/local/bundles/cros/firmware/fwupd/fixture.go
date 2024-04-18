// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fwupd

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/shutil"
	"go.chromium.org/tast/core/testing"
)

// Daemon restart is needed to drop the fake camera version and restore the internal
// state of fwupd prior the test.
// Refresh metadata prior the test suite.

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: "prepareFwupd",
		Desc: "Restart fwupd daemon when needed, and do metadata refresh",
		// ChromeOS > Platform > Services > Peripherals > Firmware Update - fwupd
		BugComponent:    "b:857851",
		Contacts:        []string{"chromeos-fwupd@google.com"},
		Impl:            &fwupdFixture{},
		SetUpTimeout:    60 * time.Second,
		PreTestTimeout:  15 * time.Second,
		TearDownTimeout: 15 * time.Second,
	})
}

type fwupdFixture struct {
	Fwupd *Fwupd
}

// FixtData shares the DBus connection and DBus object for fwupd daemon.
type FixtData struct {
	Fwupd *Fwupd
}

func (f *fwupdFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	var err error
	if f.Fwupd, err = New(); err != nil {
		s.Fatal("Failed to connect to fwupd: ", err)
	}

	// Refresh LVFS metadata.
	cmd := testexec.CommandContext(ctx, "/usr/bin/fwupdmgr", "refresh", "--force", "--json")

	if _, err := cmd.Output(testexec.DumpLogOnError); err != nil {
		s.Errorf("Metadata refresh failed: %q: %v", shutil.EscapeSlice(cmd.Args), err)
	}
	return &FixtData{Fwupd: f.Fwupd}
}

func (f *fwupdFixture) Reset(ctx context.Context) error {
	return nil
}

func (f *fwupdFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	s.Log("Restarting fwupd")
	if err := f.Fwupd.RestartDaemon(ctx); err != nil {
		s.Fatal("Failed to restart fwupd: ", err)
	}
}

func (f *fwupdFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}

func (f *fwupdFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	s.Log("Restarting fwupd")
	if err := f.Fwupd.RestartDaemon(ctx); err != nil {
		s.Fatal("Failed to restart fwupd: ", err)
	}
}
