// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"
	"time"

	hwsecremote "go.chromium.org/tast-tests/cros/remote/hwsec"
	"go.chromium.org/tast/core/testing"
)

const (
	fixtureSetUpTimeout = 3 * time.Minute

	rebootFixtureName = "rebootFixture"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: rebootFixtureName,
		// TODO(b/294473939): Make this fixture specialized for fingerprint tests, and only
		// reboot during SetUp if it's necessary. At this moment we don't have a method to
		// detect that.
		Desc: "Reboots the DUT",
		Contacts: []string{
			"hcyang@google.com",
			"cryptohome-core@google.com",
		},
		SetUpTimeout: fixtureSetUpTimeout,
		Impl:         &rebootFixtureImpl{},
	})
}

type rebootFixtureImpl struct{}

func (f *rebootFixtureImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	cmdRunner := hwsecremote.NewCmdRunner(s.DUT())
	helper, err := hwsecremote.NewHelper(cmdRunner, s.DUT())
	if err != nil {
		s.Fatal("Failed to create hwsec remote helper: ", err)
	}

	// Use hwsecremote's helper to reboot the DUT so that it can wait until the D-Bus hwsec
	// interested in are available.
	if err := helper.Reboot(ctx); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}
	return nil
}

func (f *rebootFixtureImpl) TearDown(ctx context.Context, s *testing.FixtState) {
}

func (f *rebootFixtureImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *rebootFixtureImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *rebootFixtureImpl) Reset(ctx context.Context) error {
	return nil
}
