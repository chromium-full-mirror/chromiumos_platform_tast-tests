// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package statefulmigration

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	powerwashTimeout = 2 * time.Minute
	migrationTimeout = 5 * time.Minute
)

// Operations define the functions that are supposed to be run prior and post migration.
type Operations struct {
	PreMigration  func(ctx context.Context, dut *dut.DUT) error
	PostMigration func(ctx context.Context, dut *dut.DUT) error
}

func isLvmStatefulPartition(ctx context.Context, dut *dut.DUT) bool {
	// Create command to get volume group name
	output, _ := dut.Conn().CommandContext(ctx, "sh", "-c", `/sbin/vgs -o vg_name --noheadings`).Output()
	if strings.TrimSpace(string(output)) != "" {
		return true
	}
	return false
}

func isExt4StatefulPartition(ctx context.Context, dut *dut.DUT) bool {
	return !isLvmStatefulPartition(ctx, dut)
}

func markForPowerwash(ctx context.Context, dut *dut.DUT, rpcHint *testing.RPCHint) error {
	if err := dut.Conn().CommandContext(ctx, "sh", "-c", `echo 'fast safe keepimg' > /mnt/stateful_partition/factory_install_reset`).Run(); err != nil {
		return errors.Wrap(err, "failed to write to powerwash file")
	}

	return nil
}

func performThinpoolMigration(ctx context.Context, dut *dut.DUT) error {
	_ = dut.Conn().CommandContext(ctx, "sh", "-c", `/usr/sbin/thinpool_migrator --enable`).Run()

	testing.ContextLog(ctx, "Performing thinpool migration on  DUT")
	if err := dut.Reboot(ctx); err != nil {
		return errors.Wrap(err, "failed to reboot the DUT after migration")
	}

	// Check that the DUT has LVM stateful partition.
	if !isLvmStatefulPartition(ctx, dut) {
		return errors.New("Device still using ext4 for stateful partition")
	}

	return nil
}

// MigrationTest drives end-to-end migration test by setting up a pre-migration state and validating that post-migration the state is consistent.
func MigrationTest(ctx context.Context, dut *dut.DUT, rpcHint *testing.RPCHint, outDir string, ops *Operations) error {
	if isLvmStatefulPartition(ctx, dut) {
		return errors.New("Stateful partition already on LVM")
	}

	if ops.PreMigration != nil {
		testing.ContextLog(ctx, "Running PreMigration")
		if err := ops.PreMigration(ctx, dut); err != nil {
			return errors.Wrap(err, "failed to run the PreUpdate operation")
		}
	}

	// Migrate the DUT.
	testing.ContextLog(ctx, "Running thinpool migration")
	if err := performThinpoolMigration(ctx, dut); err != nil {
		return errors.Wrap(err, "failed to migrate device to thinpool")
	}

	defer markForPowerwash(ctx, dut, rpcHint)

	if ops.PostMigration != nil {
		testing.ContextLog(ctx, "Running PostMigration")
		if err := ops.PostMigration(ctx, dut); err != nil {
			return errors.Wrap(err, "failed to run the PostMigration operation")
		}
	}

	return nil
}
