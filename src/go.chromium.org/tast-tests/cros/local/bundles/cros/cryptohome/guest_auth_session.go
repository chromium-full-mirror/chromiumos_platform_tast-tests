// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"
	"time"

	uda "go.chromium.org/chromiumos/system_api/user_data_auth_proto"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: GuestAuthSession,
		Desc: "Test guest sessions with auth session API",
		Contacts: []string{
			"cryptohome-core@google.com",
			"dlunev@chromium.org",
			"hardikgoyal@chromium.org",
		},
		BugComponent: "b:1088399", // ChromeOS > Security > Cryptohome
		Attr:         []string{"group:mainline", "group:cryptohome"},
	})
}

func GuestAuthSession(ctx context.Context, s *testing.State) {
	const (
		userName      = "foo@bar.baz"
		userPassword  = "secret"
		passwordLabel = "password"
	)

	ctxForCleanUp := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cmdRunner := hwseclocal.NewCmdRunner()
	client := hwsec.NewCryptohomeClient(cmdRunner)

	// Ensure cryptohomed is started and wait for it to be available.
	if err := cryptohome.CheckService(ctx); err != nil {
		s.Fatal("Failed to ensure cryptohomed: ", err)
	}

	if err := client.UnmountAll(ctx); err != nil {
		s.Fatal("Failed to unmount vaults for preparation: ", err)
	}

	// Phase 1: Check that guest vaults are non-persistent.

	// Set up a guest session
	if _, err := client.PrepareGuestVault(ctx); err != nil {
		s.Fatal("Failed to prepare guest vault: ", err)
	}
	defer client.UnmountAll(ctxForCleanUp)

	if _, err := client.PrepareGuestVault(ctx); err == nil {
		s.Fatal("Secondary guest attempt should fail, but succeeded")
	}

	// Write a test file to verify non-persistence.
	if err := cryptohome.WriteFileForPersistence(ctx, cryptohome.GuestUser); err != nil {
		s.Fatal("Failed to write test file: ", err)
	}

	// Unmount and mount again.
	if err := client.UnmountAll(ctx); err != nil {
		s.Fatal("Failed to unmount vaults for re-mounting: ", err)
	}

	if _, err := client.PrepareGuestVault(ctx); err != nil {
		s.Fatal("Failed to prepare guest vault: ", err)
	}

	// Verify non-persistence.
	if err := cryptohome.VerifyFileUnreadability(ctx, cryptohome.GuestUser); err != nil {
		s.Fatal("File is persisted across guest session boundary")
	}

	// Phase 2: Check that guest vaults are not mounted when another session is active.
	if err := client.UnmountAndRemoveVault(ctx, userName); err != nil {
		s.Fatal("Failed to remove vault: ", err)
	}

	if err := client.MountVault(ctx, passwordLabel, hwsec.NewPassAuthConfig(userName, userPassword), true, hwsec.NewVaultConfig()); err != nil {
		s.Fatal("Failed to create user: ", err)
	}
	defer client.UnmountAndRemoveVault(ctxForCleanUp, userName)

	reply, err := client.PrepareGuestVault(ctx)
	if err == nil {
		s.Fatal("PrepareGuestVault succeeded when there are active sessions")
	}
	if err := hwsec.CheckForPossibleAction(reply.ErrorInfo, uda.PossibleAction_POSSIBLY_REBOOT); err != nil {
		s.Error("PrepareGuestVault() when there are active sessions doesn't recommend reboot: ", err)
	}
}
