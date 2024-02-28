// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package updateutil

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: fixture.UpdateEngine,
		Desc: "Fixture for tests that use update engine, ensures status is reset",
		Contacts: []string{
			"kimjae@google.com",
			"chromeos-core-services@google.com", // Update engine
			"chromeos-commercial-remote-management@google.com",
		},
		Impl:            &updateEngineFixture{},
		PreTestTimeout:  30 * time.Second,
		PostTestTimeout: 30 * time.Second,
		ServiceDeps: []string{
			"tast.cros.autoupdate.UpdateService",
		},
	})

	// UpdateEngineEnrolled is identical to UpdateEngine but it also provides
	// enrollment.
	testing.AddFixture(&testing.Fixture{
		Name: fixture.UpdateEngineEnrolled,
		Desc: "Fixture providing enrollment and udpate engine reset",
		Contacts: []string{
			"mpolzer@chromium.org",
			"chromeos-commercial-remote-management@google.com",
		},
		Parent:          fixture.Enrolled, // Provides enrollment.
		Impl:            &updateEngineFixture{},
		PreTestTimeout:  30 * time.Second,
		PostTestTimeout: 30 * time.Second,
		ServiceDeps: []string{
			"tast.cros.autoupdate.UpdateService",
		},
	})

	// UpdateEngineCleanOwnership is a combination of CleanOwnership with UpdateEngine
	// on top of it, to take care of update_engine daemon cleanup.
	testing.AddFixture(&testing.Fixture{
		Name: fixture.UpdateEngineCleanOwnership,
		Desc: "Fixture providing clean ownership and udpate engine reset",
		Contacts: []string{
			"igorcov@chromium.org",
			"chromeos-commercial-remote-management@google.com",
		},
		Parent:          fixture.CleanOwnership, // Clean device ownership.
		Impl:            &updateEngineFixture{},
		PreTestTimeout:  30 * time.Second,
		PostTestTimeout: 30 * time.Second,
		ServiceDeps: []string{
			"tast.cros.autoupdate.UpdateService",
		},
	})
}

type updateEngineFixture struct{}

// PreTest ensures update engine is ready and idle.
func (*updateEngineFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	s.Log("UpdateEngine Fixture PreTest")
	if err := EnsureUpdateStatusIdle(ctx, s.DUT(), s.RPCHint()); err != nil {
		// If a failure is triggered, ensure the previous tests in the suite reset
		// update engine successfully after an update attempt. See b/239680170.
		s.Fatal("Update engine is not ready, did a previous test not clean up properly?: ", err)
	}
}

// PostTest ensures that the state of update engine is reset.
func (*updateEngineFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	s.Log("UpdateEngine Fixture PostTest")

	logTarget := filepath.Join(s.OutDir(), "post_test_update_engine.log")
	if err := linuxssh.GetFile(ctx, s.DUT().Conn(), "/var/log/update_engine.log", logTarget, linuxssh.DereferenceSymlinks); err != nil {
		s.Errorf("Failed to copy update engine log to %q: %v", logTarget, err)
	}

	if err := ResetUpdateStatus(ctx, s.DUT(), s.RPCHint()); err != nil {
		s.Fatal("Failed to reset update status: ", err)
	}
}

func (*updateEngineFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	return s.ParentValue()
}
func (*updateEngineFixture) Reset(ctx context.Context) error                    { return nil }
func (*updateEngineFixture) TearDown(ctx context.Context, s *testing.FixtState) {}
