// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/cryptohome/cleanup"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: AutomaticCleanup,
		Desc: "Test automatic disk cleanup",

		BugComponent: "b:1264602", // ChromeOS > Software > Commercial (Enterprise) > DiskCleanup
		Contacts: []string{
			"chromeos-disk-cleanup@google.com",
			"vsavu@google.com",     // Test author
			"gwendal@chromium.org", // Lead for ChromeOS Storage
		},
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
		},
	})
}

func AutomaticCleanup(ctx context.Context, s *testing.State) {
	const (
		homedirSize = 100 * cleanup.MiB // 100 Mib, used for testing

		temporaryUser = "tmp-user"
		user1         = "cleanup-user1"
		user2         = "cleanup-user2"
		password      = "1234"
	)

	cmdRunner := hwseclocal.NewCmdRunner()
	helper, err := hwseclocal.NewHelper(cmdRunner)
	if err != nil {
		s.Fatal("Failed to create hwsec local helper: ", err)
	}
	daemonController := helper.DaemonController()

	// Start cryptohomed and wait for it to be available
	if err := daemonController.Ensure(ctx, hwsec.CryptohomeDaemon); err != nil {
		s.Fatal("Failed to start cryptohomed: ", err)
	}

	defer daemonController.Restart(ctx, hwsec.CryptohomeDaemon)

	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	if err := cleanup.RunOnExistingUsers(ctx); err != nil {
		s.Fatal("Failed to perform initial cleanup: ", err)
	}

	// Create users with contents to fill up disk space
	fillFile1, err := cleanup.CreateFilledUserHomedir(ctx, user1, password, "Cache", homedirSize)
	if err != nil {
		s.Fatal("Failed to create user with content: ", err)
	}
	defer cryptohome.RemoveVault(ctx, user1)

	fillFile2, err := cleanup.CreateFilledUserHomedir(ctx, user2, password, "Cache", homedirSize)
	if err != nil {
		s.Fatal("Failed to create user with content: ", err)
	}
	defer cryptohome.RemoveVault(ctx, user2)
	// Unmount all users before removal.
	defer cryptohome.UnmountAll(ctx)

	// Unmount just the first user
	if err := cryptohome.UnmountVault(ctx, user1); err != nil {
		s.Fatal("Failed to unmount user vault: ", err)
	}

	// Remount the second user
	if err := cryptohome.MountVault(ctx, user2, password); err != nil {
		s.Fatal("Failed to remount user vault: ", err)
	}

	if err := cryptohome.WaitForUserMount(ctx, user2); err != nil {
		s.Fatal("Failed to remount user vault: ", err)
	}

	if err := cleanup.ForceAutomaticCleanup(ctx); err != nil {
		s.Fatal("Failed to run cleanup: ", err)
	}

	if _, err := os.Stat(fillFile1); err == nil {
		s.Error("fillFile for user1 still present")
	} else if !os.IsNotExist(err) {
		s.Fatal("Failed to check if fill file exists: ", err)
	}

	if _, err := os.Stat(fillFile2); err == nil {
		s.Error("fillFile for user2 still present")
	} else if !os.IsNotExist(err) {
		s.Fatal("Failed to check if fill file exists: ", err)
	}
}
