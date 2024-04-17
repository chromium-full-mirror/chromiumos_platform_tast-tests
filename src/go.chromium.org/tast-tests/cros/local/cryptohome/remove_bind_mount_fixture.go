// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"
	"time"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: "removeBindMountFixture",
		Desc: "Set up Cryptohomed to run without downloads_bind_mount",
		Contacts: []string{
			"wenbojie@chromium.org",
			"chromeos-files-syd@google.com",
		},
		BugComponent:    "b:167289",
		SetUpTimeout:    fixtureSetUpTimeout,
		ResetTimeout:    fixtureResetTimeout,
		TearDownTimeout: fixtureTearDownTimeout,
		PreTestTimeout:  30 * time.Second,
		Impl:            &removeBindMountFixtureImpl{},
	})
}

type removeBindMountFixtureImpl struct{}

func (f *removeBindMountFixtureImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	// Start cryptohomed and wait for it to be available.
	if err := CheckService(ctx); err != nil {
		s.Fatal("Failed to start cryptohomed: ", err)
	}

	// Restart cryptohomed with no_downloads_bind_mount.
	if err := RestartCryptohomed(ctx, false /*enableDownloadsBindMount*/); err != nil {
		s.Fatal("Failed to restart cryptohomed to remove downloads bind mount: ", err)
	}
	return nil
}

func (f *removeBindMountFixtureImpl) TearDown(ctx context.Context, s *testing.FixtState) {
	// Clean up obsolete states, in case there's any.
	if err := UnmountAll(ctx); err != nil {
		s.Fatal()
	}
	// Restart cryptohomed with downloads_bind_mount.
	if err := RestartCryptohomed(ctx, true /*enableDownloadsBindMount*/); err != nil {
		s.Fatal("Failed to restart cryptohomed to enable downloads bind mount: ", err)
	}
}

func (f *removeBindMountFixtureImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
	// Assert there is no bind mount for Downloads.
	existAfterMigration, err := DownloadsExistAsBindMount(ctx)
	if err != nil {
		s.Fatal("Failed to get mount information: ", err)
	}
	if existAfterMigration {
		s.Fatal("Failed to remove downloads bind mount")
	}
}

func (f *removeBindMountFixtureImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *removeBindMountFixtureImpl) Reset(ctx context.Context) error {
	return nil
}
