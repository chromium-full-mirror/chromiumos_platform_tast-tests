// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast/core/testing"
)

const (
	fixtureSetUpTimeout    = 1 * time.Minute
	fixtureResetTimeout    = 1 * time.Minute
	fixtureTearDownTimeout = 1 * time.Minute

	ussAuthSessionFixtureName = "ussAuthSessionFixture"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: ussAuthSessionFixtureName,
		Desc: "Set up the USS flag experiement flag for Auth Session",
		Contacts: []string{
			"lziest@google.com",
			"cryptohome-core@google.com",
		},
		SetUpTimeout:    fixtureSetUpTimeout,
		ResetTimeout:    fixtureResetTimeout,
		TearDownTimeout: fixtureTearDownTimeout,
		Impl:            &fixtureImpl{},
	})
}

type cleanupFunc func(context.Context) error

type fixtureImpl struct {
	ussFlagCleanup        cleanupFunc
	ussDisableFlagCleanup cleanupFunc
}

// AuthSessionFixture provides data on how the session has been configured by the fixture.
type AuthSessionFixture struct {
	UssEnabled bool
}

func (f *fixtureImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	cmdRunner := hwseclocal.NewCmdRunner()
	helper, err := hwseclocal.NewHelper(cmdRunner)
	if err != nil {
		s.Fatal("Failed to create hwsec local helper: ", err)
	}

	// Wait for cryptohomed becomes available.
	daemonController := helper.DaemonController()
	if err := daemonController.Ensure(ctx, hwsec.CryptohomeDaemon); err != nil {
		s.Fatal("Failed to ensure cryptohomed: ", err)
	}
	if err := UnmountAll(ctx); err != nil {
		s.Fatal("Failed to unmount all: ", err)
	}

	// Enable the UserSecretStash experiment for the duration of the test by
	// creating a flag file that's checked by cryptohomed.
	// A cleanup routine is returned by the helper function. We will run it
	// when tearing down the test environment.
	f.ussFlagCleanup, err = helper.EnableUserSecretStash(ctx)
	if err != nil {
		s.Fatal("Failed to enable the UserSecretStash experiment: ", err)
	}
	return &AuthSessionFixture{
		UssEnabled: true,
	}
}

func (f *fixtureImpl) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := UnmountAll(ctx); err != nil {
		s.Error("Failed to unmount all: ", err)
	}
	err := f.ussFlagCleanup(ctx)
	if err != nil {
		s.Error("Failed to clean up the USS flag: ", err)
	}
}

func (f *fixtureImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *fixtureImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *fixtureImpl) Reset(ctx context.Context) error {
	// Clean up obsolete state, in case there's any.
	if err := UnmountAll(ctx); err != nil {
		return err
	}
	return nil
}
