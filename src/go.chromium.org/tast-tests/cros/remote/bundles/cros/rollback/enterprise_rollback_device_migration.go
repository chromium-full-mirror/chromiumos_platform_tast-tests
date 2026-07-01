// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rollback

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/remote/rollback"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: EnterpriseRollbackDeviceMigration,
		Desc: "Verify that networks are preserved during device migration",
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"aidazolic@google.com", // Test author
		},
		BugComponent: "b:1031231",
		Attr: []string{
			"group:powerwash-daily",
		},
		SoftwareDeps: []string{"reboot", "chrome"},
		ServiceDeps: []string{
			"tast.cros.hwsec.OwnershipService",
			"tast.cros.rollback.EnterpriseRollbackService",
		},
		Timeout: 10 * time.Minute,
	})
}

// EnterpriseRollbackDeviceMigration does not expect to use enrollment so any
// functionality that depend on the enrollment of the device should be not be
// added to this test. It simulates the device migration use case, which
// triggers the same data save as an OS downgrade.
func EnterpriseRollbackDeviceMigration(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Minute)
	defer cancel()

	defer func(ctx context.Context) {
		if err := rollback.ClearRollbackAndSystemData(ctx, s.DUT(), s.RPCHint()); err != nil {
			s.Error("Failed to clean rollback data after test: ", err)
		}
	}(cleanupCtx)

	if err := rollback.ClearRollbackAndSystemData(ctx, s.DUT(), s.RPCHint()); err != nil {
		s.Fatal("Failed to clean rollback data before test: ", err)
	}

	networksInfo, err := rollback.ConfigureNetworks(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to configure networks: ", err)
	}

	// First write the device migration trigger flag, which tells oobe_config_save
	// that this is a device migration, not a normal rollback.
	deviceMigrationFlag := "/mnt/stateful_partition/unencrypted/preserve/.save_device_migration_data"
	if err := s.DUT().Conn().CommandContext(ctx, "touch", deviceMigrationFlag).Run(); err != nil {
		s.Fatal("Failed to write device migration flag: ", err)
	}

	// And then save the rollback data - this triggers oobe_config_save.
	sensitive, err := rollback.SaveRollbackData(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to save rollback data: ", err)
	}

	// Ineffective reset is ok here as the device steps through oobe automatically.
	s.Log("Simulating powerwash and rebooting the DUT to fake a device migration")
	if err := rollback.SimulatePowerwashAndReboot(ctx, s.DUT()); err != nil && !errors.Is(err, hwsec.ErrIneffectiveReset) {
		s.Fatal("Failed to simulate powerwash and reboot: ", err)
	}

	// Verify migration data, which is the same as rollback data.
	if err := rollback.VerifyRollbackData(ctx, s.DUT(), s.RPCHint(), networksInfo, sensitive); err != nil {
		s.Fatal("Failed to verify migration data: ", err)
	}
}
