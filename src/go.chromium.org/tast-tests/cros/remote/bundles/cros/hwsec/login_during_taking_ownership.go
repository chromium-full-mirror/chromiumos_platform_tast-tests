// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	hwsecremote "go.chromium.org/tast-tests/cros/remote/hwsec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LoginDuringTakingOwnership,
		Desc:         "Verifies that login is workin during TPM ownership is being taken",
		Contacts:     []string{"cros-hwsec@google.com", "yich@google.com"},
		BugComponent: "b:1188704",
		SoftwareDeps: []string{"reboot", "tpm_clear_allowed"},
		Attr:         []string{"group:hwsec_destructive_func"},
	})
}

func LoginDuringTakingOwnership(ctx context.Context, s *testing.State) {
	cmdRunner := hwsecremote.NewCmdRunner(s.DUT())

	helper, err := hwsecremote.NewHelper(cmdRunner, s.DUT())
	if err != nil {
		s.Fatal("Helper creation error: ", err)
	}

	utility := helper.CryptohomeClient()

	if err := helper.EnsureTPMAndSystemStateAreReset(ctx); err != nil {
		s.Fatal("Failed to ensure resetting TPM: ", err)
	}

	loginErr := make(chan error)
	loginRoutine := func(username, passwd string) {
		loginErr <- func() error {
			if err := utility.MountVault(ctx, "dontcare", hwsec.NewPassAuthConfig(username, passwd), true, hwsec.NewVaultConfig()); err != nil {
				return errors.Wrap(err, "error during create vault")
			}
			return nil
		}()
	}

	ownershipErr := make(chan error)
	takeOwnershipRoutine := func() {
		ownershipErr <- func() error {
			if err := helper.EnsureTPMIsReady(ctx, hwsec.DefaultTakingOwnershipTimeout); err != nil {
				return errors.Wrap(err, "failed to ensure ownership")
			}
			return nil
		}()
	}

	const username = "unowned@gmail.com"
	const passwd = "passwd"
	go loginRoutine(username, passwd)
	go takeOwnershipRoutine()

	for i := 0; i < 2; i++ {
		select {
		case err := <-loginErr:
			if err != nil {
				s.Error("login error: ", err)
			}
		case err := <-ownershipErr:
			if err != nil {
				s.Error("ownership error: ", err)
			}
		}
	}

	result, err := utility.Unmount(ctx, username)
	if err != nil {
		s.Fatal("Error unmounting user: ", err)
	}
	if !result {
		s.Fatal("Failed to unmount user: ", err)
	}
	result, err = utility.RemoveVault(ctx, username)
	if err != nil {
		s.Fatal("Error removing vault: ", err)
	}
	if !result {
		s.Fatal("Failed to remove vault: ", err)
	}
}
