// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast/core/testing"
)

const (
	// Well known dbus name for biod.
	dbusName = "org.chromium.BiometricsDaemon"

	fakeBiometricsFixtureName = "fakeBiometricsFixture"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: fakeBiometricsFixtureName,
		Desc: "Set up AuthSession With Fake Biometrics Daemon",
		Contacts: []string{
			"lziest@google.com",
			"cryptohome-core@google.com",
		},
		SetUpTimeout:    fixtureSetUpTimeout,
		ResetTimeout:    fixtureResetTimeout,
		TearDownTimeout: fixtureTearDownTimeout,
		Impl:            &biometricsFixtureImpl{},
		// This fixture needs to be set up after establishing the PinWeaver pairing key because the
		// protocol won't work without it.
		Parent: "establishPwPairingKeyFixture",
	})
}

type biometricsFixtureImpl struct {
	FakeAuthStackManager *FakeAuthStackManager
}

// BiometricsFixture provides data on how the biometrics related components is configured.
type BiometricsFixture struct {
	FakeAuthStackManager *FakeAuthStackManager
}

func (f *biometricsFixtureImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	cmdRunner := hwseclocal.NewCmdRunner()
	helper, err := hwseclocal.NewHelper(cmdRunner)
	if err != nil {
		s.Fatal("Failed to create hwsec local helper: ", err)
	}

	// Stop the original biod so fake biod can start and take ownership of DBus connections.
	daemonController := helper.DaemonController()
	if err := daemonController.TryStop(ctx, hwsec.BiometricsDaemon); err != nil {
		s.Fatal("Failed to stop biod: ", err)
	}

	// Start the fake auth stack manager that listens on the biod dbus connection.
	dbusConn, err := dbusutil.SystemBus()
	if err != nil {
		s.Fatal("Failed to connect to system D-Bus bus: ", err)
	}
	if _, err := dbusConn.RequestName(dbusName, 0 /* flags */); err != nil {
		s.Fatal("Failed to request the well-known D-Bus name: ", err)
	}
	fasm, err := NewFakeAuthStackManager(ctx, dbusConn)
	if err != nil {
		s.Fatal("Failed to initialize FakeAuthStackManager: ", err)
	}
	f.FakeAuthStackManager = fasm

	return &BiometricsFixture{
		FakeAuthStackManager: fasm,
	}
}

func (f *biometricsFixtureImpl) TearDown(ctx context.Context, s *testing.FixtState) {
	// Clean up test user states, in case there's any.
	if err := UnmountAll(ctx); err != nil {
		s.Error("Failed to unmount all: ", err)
	}
	// Restore biod process if it is available.
	cmdRunner := hwseclocal.NewLoglessCmdRunner()
	helper, err := hwseclocal.NewHelper(cmdRunner)
	if err != nil {
		s.Error("Failed to create hwsec local helper: ", err)
	}
	err = helper.DaemonController().Ensure(ctx, hwsec.BiometricsDaemon)
	if err != nil {
		s.Error("Failed to ensure original biod is restored: ", err)
	}
	if f.FakeAuthStackManager == nil {
		return
	}
	f.FakeAuthStackManager.Close()
	if _, err := f.FakeAuthStackManager.dbusConn.ReleaseName(dbusName); err != nil {
		s.Fatal("Failed to release biod dbus connection: ", err)
	}
}

func (f *biometricsFixtureImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *biometricsFixtureImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *biometricsFixtureImpl) Reset(ctx context.Context) error {
	// Clean up obsolete states, in case there's any.
	if err := UnmountAll(ctx); err != nil {
		return err
	}
	return nil
}
