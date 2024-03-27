// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/hwsec/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/hwsec/util"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrossVersionPinWeaver,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies functionality of pinweaver operations across versions",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		// ChromeOS > Platform > System > Hardware Security > HwSec AP
		BugComponent: "b:1188704",
		Attr:         []string{"group:hw_agnostic"},
		// TODO(b/249934249): Add the gsc version test after we enabled the ti50-emulator.
		SoftwareDeps: []string{"tpm2_simulator"},
		Params: []testing.Param{
			{
				Name:      "current",
				Fixture:   "crossVersion.current",
				ExtraAttr: []string{"group:mainline", "informational"},
			},
		},
	})
}

func testPinWeaverLogReplay(ctx context.Context, helper hwsec.CmdHelper, config *util.CrossVersionLoginConfig) error {
	pinweaver := helper.PinWeaverManagerClient()

	// Log Replay Case #1 [lost check(failed) + check(ok) on different label]
	if err := helper.RestorePinWeaverHashTreeSnapShot(ctx, 0); err != nil {
		return err
	}
	if err := helper.RestoreTpmNVChipSnapShot(ctx, 1); err != nil {
		return err
	}
	if err := pinweaver.SyncHashTree(ctx); err != nil {
		return errors.Wrap(err, "failed replay log test #1 [lost insert(failed) + insert(ok) on different label]")
	}

	// Log Replay Case #2 [lost insert + remove on same label]
	if err := helper.RestoreTpmNVChipSnapShot(ctx, 2); err != nil {
		return err
	}
	if err := pinweaver.SyncHashTree(ctx); err != nil {
		return errors.Wrap(err, "failed replay log test #2 [lost insert + remove on same label]")
	}

	// Log Replay Case #3 [lost removes]
	if err := helper.RestoreTpmNVChipSnapShot(ctx, 3); err != nil {
		return err
	}
	if err := pinweaver.SyncHashTree(ctx); err != nil {
		return errors.Wrap(err, "failed replay log test #3 [lost removes]")
	}

	// Log Replay Case #4 [lost inserts]
	if err := helper.RestoreTpmNVChipSnapShot(ctx, 4); err != nil {
		return err
	}
	if err := pinweaver.SyncHashTree(ctx); err != nil {
		return errors.Wrap(err, "failed replay log test #4 [lost inserts]")
	}
	return nil
}

func testPinWeaver(ctx context.Context, helper hwsec.CmdHelper, config *util.CrossVersionLoginConfig) error {
	normalLabels := config.PinWeaverLabels.NormalLabels
	lockedOutLabels := config.PinWeaverLabels.LockedOutLabels
	const usedNormalLabelCount = 3
	const usedLockedOutLabelCount = 4

	pinweaver := helper.PinWeaverManagerClient()

	if len(normalLabels) < usedNormalLabelCount || len(lockedOutLabels) < usedLockedOutLabelCount {
		return errors.New("PinWeaver labels prepared before migration aren't enough")
	}

	// Test1: check a normal label with correct secret should succeed
	_, _, err := pinweaver.CheckCredential(ctx, normalLabels[0], hwsec.DefaultLeSecret)
	if err != nil {
		return errors.Wrap(err, "failed to check a label that is created before migration")
	}

	// Test2: check a normal label with wrong sercet should fail
	_, _, err = pinweaver.CheckCredential(ctx, normalLabels[1], hwsec.WrongSecret)
	if err == nil {
		return errors.Wrap(err, "incorrectly succeeded to check a label that is created before migration with wrong secret")
	}

	// Test3: check a normal label can be removed
	if err = pinweaver.RemoveCredential(ctx, normalLabels[2]); err != nil {
		return errors.Wrap(err, "failed to remove a label that is created before migration")
	}

	// Test4: check a locked-out label with correct secret should fail
	_, _, err = pinweaver.CheckCredential(ctx, lockedOutLabels[0], hwsec.DefaultLeSecret)
	if err == nil {
		return errors.Wrap(err, "incorrectly succeeded to check a label that is locked before migration")
	}

	// Test5: reset a locked-out label with correct secret should succeed. Additionally verify that the label is indeed not locked out.
	if err = pinweaver.ResetCredential(ctx, lockedOutLabels[1], hwsec.DefaultResetSecret); err != nil {
		return errors.Wrap(err, "failed to reset a label that is created before migration")
	}
	_, _, err = pinweaver.CheckCredential(ctx, lockedOutLabels[1], hwsec.DefaultLeSecret)
	if err != nil {
		return errors.Wrap(err, "failed to check a label that is already reseted")
	}

	// Test6: reset a locked-out label with wrong secret should fail. Additionally verify that the label is still locked out.
	if err = pinweaver.ResetCredential(ctx, lockedOutLabels[2], hwsec.WrongSecret); err == nil {
		return errors.Wrap(err, "incorrectly succeeded to reset a label that is created before migration with wrong secret")
	}
	_, _, err = pinweaver.CheckCredential(ctx, lockedOutLabels[2], hwsec.DefaultLeSecret)
	if err == nil {
		return errors.Wrap(err, "incorrectly succeeded to check a label that is locked before migration")
	}

	// Test7: check a locked-out label can be removed
	if err = pinweaver.RemoveCredential(ctx, lockedOutLabels[3]); err != nil {
		return errors.Wrap(err, "failed to remove a label that is created before migration")
	}

	return testPinWeaverLogReplay(ctx, helper, config)
}

func CrossVersionPinWeaver(ctx context.Context, s *testing.State) {
	cmdRunner := hwseclocal.NewCmdRunner()
	helper, err := hwseclocal.NewHelper(cmdRunner)
	if err != nil {
		s.Fatal("Failed to create hwsec local helper: ", err)
	}

	fixtureData := s.FixtValue().(*fixture.CrossVersionLoginFixture)
	for _, config := range fixtureData.ConfigList {
		if config.PinWeaverLabels == nil {
			continue
		}
		if err := testPinWeaver(ctx, helper.CmdHelper, &config); err != nil {
			s.Fatal("Failed to test pinweaver: ", err)
		}
	}
}
