// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"
	"math/rand"
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

var usernamePool = []string{"foo@bar.baz", "test@gmail.com"}

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

type fixtureImpl struct {
	testUserName   string
	ussFlagCleanup hwsec.CleanupFunc
}

// AuthSessionFixture provides data on how the session has been configured by the fixture.
type AuthSessionFixture struct {
	TestUserName string
	UssEnabled   bool
}

func (f *fixtureImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	// Wait for cryptohomed becomes available.
	if err := CheckService(ctx); err != nil {
		s.Fatal("Failed to ensure cryptohomed: ", err)
	}

	// Pick a user name, and ensure it is a clean slate to start a test.
	testUserName := usernamePool[rand.Intn(len(usernamePool))]
	f.testUserName = testUserName
	// Remove test user vault as a previous
	// test may crash and have an old user directory partially setup.
	if err := ForceRemoveVault(ctx, testUserName); err != nil {
		s.Fatal("Failed to remove old test user vault: ", err)
	}

	// Enable the UserSecretStash experiment for the duration of the test by
	// creating a flag file that's checked by cryptohomed.
	// A cleanup routine is returned by the helper function. We will run it
	// when tearing down the test environment.
	helper, err := hwseclocal.NewHelper(hwseclocal.NewCmdRunner())
	if err != nil {
		s.Fatal("Failed to create hwsec local helper: ", err)
	}
	f.ussFlagCleanup, err = helper.EnableUserSecretStash(ctx)
	if err != nil {
		s.Fatal("Failed to enable the UserSecretStash experiment: ", err)
	}
	return &AuthSessionFixture{
		TestUserName: testUserName,
		UssEnabled:   true,
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
	// After each test, clean up by removing user vault.
	if f.testUserName != "" {
		if err := ForceRemoveVault(ctx, f.testUserName); err != nil {
			return err
		}
	}
	// Ensure cryptohomed is running.
	if err := CheckService(ctx); err != nil {
		return err
	}
	return nil
}
