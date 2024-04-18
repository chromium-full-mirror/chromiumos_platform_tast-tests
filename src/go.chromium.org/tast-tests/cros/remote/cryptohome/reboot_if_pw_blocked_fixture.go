// Copyright 2024 The ChromiumOS Authors
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

	rebootIfPwBlockedFixtureName = "rebootIfPwBlockedFixture"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: rebootIfPwBlockedFixtureName,
		Desc: "Reboots the DUT if PinWeaver pairing key establishment operation is blocked due to user login",
		Contacts: []string{
			"hcyang@google.com",
			"cryptohome-core@google.com",
		},
		BugComponent: "b:1188704",
		SetUpTimeout: fixtureSetUpTimeout,
		Impl:         &rebootIfPwBlockedFixture{},
	})
}

type rebootIfPwBlockedFixture struct{}

func (f *rebootIfPwBlockedFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	cmdRunner := hwsecremote.NewCmdRunner(s.DUT())
	helper, err := hwsecremote.NewHelper(cmdRunner, s.DUT())
	if err != nil {
		s.Fatal("Failed to create hwsec remote helper: ", err)
	}
	cryptohomeHelper := helper.CryptohomeClient()

	_, err = cmdRunner.Run(ctx, "cat", "/var/lib/biod/CrosFpAuthStackManager/wrapped_pk")
	if err == nil {
		// Pk is already established on this device.
		return nil
	}

	// Else, Pk isn't established on this device. We need to check whether establishment is
	// blocked at the moment, and reboot if so.
	blocked, err := cryptohomeHelper.IsPinWeaverPkEstablishmentBlocked(ctx)
	if err != nil {
		testing.ContextLog(ctx, "Failed to call IsPinWeaverPkEstablishmentBlocked, treat it as blocked and proceed to reboot")
	}
	if blocked || err != nil {
		// Use hwsecremote's helper to reboot the DUT so that it can wait until the D-Bus hwsec
		// interested in are available.
		if err := helper.Reboot(ctx); err != nil {
			s.Fatal("Failed to reboot: ", err)
		}
	}
	return nil
}

func (f *rebootIfPwBlockedFixture) TearDown(ctx context.Context, s *testing.FixtState) {
}

func (f *rebootIfPwBlockedFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *rebootIfPwBlockedFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *rebootIfPwBlockedFixture) Reset(ctx context.Context) error {
	return nil
}
