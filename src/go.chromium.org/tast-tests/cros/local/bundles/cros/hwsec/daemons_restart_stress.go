// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: DaemonsRestartStress,
		Desc: "Verifies that restarting hwsec daemons wouldn't cause problems",
		Contacts: []string{
			"cros-hwsec@google.com",
			"yich@chromium.org",
		},
		BugComponent: "b:1188704",
		SoftwareDeps: []string{"tpm"},
		Attr:         []string{"group:attestation", "group:bootlockbox", "group:chaps", "group:cryptohome", "group:hwsec_infra", "group:tpm_manager", "group:u2fd", "group:vtpm"},
		Params: []testing.Param{{
			Name:              "tpm1",
			ExtraSoftwareDeps: []string{"tpm1", "no_tpm_dynamic"},
			ExtraAttr:         []string{"group:hwsec", "hwsec_nightly"},
			Timeout:           4 * time.Minute,
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"tpm2", "qemu"},
			ExtraAttr:         []string{"group:hwsec", "hwsec_nightly"},
			Timeout:           4 * time.Minute,
		}, {
			Name:              "tpm2",
			ExtraSoftwareDeps: []string{"tpm2", "no_qemu", "no_tpm_dynamic"},
			ExtraAttr:         []string{"group:mainline"},
		}, {
			Name:              "tpm_dynamic_1",
			ExtraSoftwareDeps: []string{"tpm_dynamic"},
			ExtraHardwareDeps: hwdep.D(hwdep.HasTpm1()),
			ExtraAttr:         []string{"group:hwsec", "hwsec_nightly"},
			Timeout:           4 * time.Minute,
		}, {
			Name:              "tpm_dynamic_2",
			ExtraSoftwareDeps: []string{"no_qemu", "tpm_dynamic"},
			ExtraHardwareDeps: hwdep.D(hwdep.HasTpm2()),
			ExtraAttr:         []string{"group:mainline"},
		}},
	})
}

// DaemonsRestartStress checks that restarting hwsec daemons wouldn't cause problems.
func DaemonsRestartStress(ctx context.Context, s *testing.State) {
	cmdRunner := hwseclocal.NewCmdRunner()
	helper, err := hwseclocal.NewHelper(cmdRunner)
	if err != nil {
		s.Fatal("Failed to create hwsec local helper: ", err)
	}
	tpmManager := helper.TPMManagerClient()
	daemonController := helper.DaemonController()

	// Check that lockout shouldn't be in effect.
	info, err := tpmManager.GetDAInfo(ctx)
	if err != nil {
		s.Fatal("Failed to get dictionary attack info: ", err)
	}
	if info.InEffect {
		s.Fatal("Lockout in effect before testing")
	}

	ctxForResumeDaemons := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	// Drop the DA reset permission.
	restorePermCall, err := helper.DropResetLockPermissions(ctx)
	if err != nil {
		// This step isn't necessary on most new devices, and on reven devices.
		// Even if it failed, it won't really affect the validity of this test.
		s.Log("Failed to drop the DA reset permission: ", err)
	} else {
		defer func(ctx context.Context) {
			// Restore the DA reset permission.
			if err = restorePermCall(ctx); err != nil {
				s.Log("Failed to restore lockout permission: ", err)
			}
			// Make sure the tpm_manager is in the stable state.
			if _, err := tpmManager.GetDAInfo(ctx); err != nil {
				s.Log("Failed to get DA info: ", err)
			}
		}(ctxForResumeDaemons)
	}

	tpmVer, err := helper.GetTPMVersion(ctx)
	if err != nil {
		s.Fatal("Failed to get TPM version: ", err)
	}

	// Restart TPM related daemons multiple times.
	for i := 0; i < 10; i++ {
		func() {
			if err := daemonController.Stop(ctx, hwsec.CryptohomeDaemon); err != nil {
				s.Fatal("Failed to stop cryptohomed: ", err)
			}
			defer func() {
				if err := daemonController.Start(ctx, hwsec.CryptohomeDaemon); err != nil {
					s.Fatal("Failed to start cryptohomed: ", err)
				}
			}()

			if err := daemonController.Stop(ctx, hwsec.AttestationDaemon); err != nil {
				s.Fatal("Failed to stop attestationd: ", err)
			}
			defer func() {
				if err := daemonController.Start(ctx, hwsec.AttestationDaemon); err != nil {
					s.Fatal("Failed to start attestationd: ", err)
				}
			}()

			if err := daemonController.Stop(ctx, hwsec.TPMManagerDaemon); err != nil {
				s.Fatal("Failed to stop tpm_managerd: ", err)
			}
			defer func() {
				if err := daemonController.Start(ctx, hwsec.TPMManagerDaemon); err != nil {
					s.Fatal("Failed to start tpm_managerd: ", err)
				}
			}()

			switch tpmVer {
			case "1.2":
				if daemonController.Restart(ctx, hwsec.TcsdDaemon); err != nil {
					s.Fatal("Failed to restart tcsd: ", err)
				}
			case "2.0":
				if daemonController.Restart(ctx, hwsec.TrunksDaemon); err != nil {
					s.Fatal("Failed to restart trunksd: ", err)
				}
			default:
				s.Fatal("Unknown TPM version: ")
			}
		}()
	}

	// Check counter didn't increase too much, and lockout shouldn't be in effect.
	newInfo, err := tpmManager.GetDAInfo(ctx)
	if err != nil {
		s.Fatal("Failed to get dictionary attack info: ", err)
	}

	// The counter shouldn't constantly increase when we restart daemons multiple time.
	// But it's acceptable to increase a little bit.
	// For example: we need one quota to check the validity of empty password.
	if newInfo.Counter > info.Counter+1 {
		s.Fatalf("Unexpected counter increase, %d -> %d", info.Counter, newInfo.Counter)
	}
	if newInfo.InEffect {
		s.Fatal("Lockout in effect after testing")
	}
}
