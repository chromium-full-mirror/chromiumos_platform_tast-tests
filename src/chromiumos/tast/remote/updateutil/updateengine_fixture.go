// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package updateutil

import (
	"context"
	"time"

	"chromiumos/tast/common/fixture"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: fixture.UpdateEngine,
		Desc: "Fixture for tests that use update engine, ensures status is reset",
		Contacts: []string{
			"crisguerrero@google.com",           // Author
			"chromeos-core-services@google.com", // Update engine
			"chromeos-commercial-remote-management@google.com",
		},
		Impl:            &updateEngineFixture{},
		PostTestTimeout: 30 * time.Second,
		ServiceDeps: []string{
			"tast.cros.autoupdate.UpdateService",
		},
	})
}

type updateEngineFixture struct{}

// PostTest ensures that the state of update engine is reset.
func (*updateEngineFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	s.Log("UpdateEngine Fixture PostTest")
	if err := ResetUpdateStatus(ctx, s.DUT(), s.RPCHint()); err != nil {
		s.Fatal("Failed to reset update status: ", err)
	}
}

func (*updateEngineFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} { return nil }
func (*updateEngineFixture) PreTest(ctx context.Context, s *testing.FixtTestState)       {}
func (*updateEngineFixture) Reset(ctx context.Context) error                             { return nil }
func (*updateEngineFixture) TearDown(ctx context.Context, s *testing.FixtState)          {}
