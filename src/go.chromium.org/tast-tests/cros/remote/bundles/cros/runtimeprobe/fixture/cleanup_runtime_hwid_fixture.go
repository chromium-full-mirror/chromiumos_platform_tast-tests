// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runtimeprobe

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: fixture.CleanupRuntimeHWID,
		Desc: "Deletes the Runtime HWID file from DUT at the beginning and end of the tests",
		Contacts: []string{
			"chromeos-runtime-probe@google.com",
			"clarkchung@google.com",
			"allenshihmc@google.com",
		},
		BugComponent:    "b:606088",
		Impl:            &cleanupRuntimeHWID{},
		PreTestTimeout:  time.Minute,
		PostTestTimeout: time.Minute,
	})
}

const runtimeHWIDFilePath = "/var/cache/hardware_verifier/runtime_hwid"

type cleanupRuntimeHWID struct{}

func (f *cleanupRuntimeHWID) SetUp(ctx context.Context, s *testing.FixtState) interface{} { return nil }

func (f *cleanupRuntimeHWID) TearDown(ctx context.Context, s *testing.FixtState) {}
func (f *cleanupRuntimeHWID) Reset(ctx context.Context) error                    { return nil }
func (f *cleanupRuntimeHWID) PreTest(ctx context.Context, s *testing.FixtTestState) {
	removeRuntimeHWIDFile(ctx, s)
}
func (f *cleanupRuntimeHWID) PostTest(ctx context.Context, s *testing.FixtTestState) {
	removeRuntimeHWIDFile(ctx, s)
}

func removeRuntimeHWIDFile(ctx context.Context, s *testing.FixtTestState) {
	if err := s.DUT().Conn().CommandContext(ctx, "rm", "-f", runtimeHWIDFilePath).Run(); err != nil {
		s.Logf("Failed to remove Runtime HWID file %q: %v", runtimeHWIDFilePath, err)
	}
}
