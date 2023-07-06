// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/common/pkcs11"
	"go.chromium.org/tast-tests/cros/common/pkcs11/pkcs11test"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/hwsec/util"
	libhwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ChapsRemount,
		Desc: "Verifies chaps works correctly after remount",
		Attr: []string{"group:hwsec", "hwsec_nightly", "group:chaps"},
		Contacts: []string{
			"cros-hwsec@google.com",
			"yich@google.com",
		},
		BugComponent: "b:1188704",
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"tpm", "no_tpm_dynamic"},
			// checkSoftwareBacked: For devices with official TPM, we do not allow fallback
			// to software backed keys so we need to check if the keys are not software backed
			// when they're not supposed to be.
			Val: true,
		}, {
			Name:              "tpm_dynamic",
			ExtraSoftwareDeps: []string{"tpm_dynamic"},
			Val:               false,
		}},
		Timeout: 4 * time.Minute,
	})
}

func ChapsRemount(ctx context.Context, s *testing.State) {
	r := libhwseclocal.NewCmdRunner()

	helper, err := libhwseclocal.NewHelper(r)
	if err != nil {
		s.Fatal("Failed to create hwsec helper: ", err)
	}
	cryptohome := helper.CryptohomeClient()

	pkcs11Util, err := pkcs11.NewChaps(ctx, r, cryptohome)
	if err != nil {
		s.Fatal("Failed to create PKCS#11 Utility: ", err)
	}

	const scratchpadPath = "/tmp/ChapsRemountTest"

	// Remove all keys/certs before the test as well.
	if err := pkcs11test.CleanupScratchpad(ctx, r, scratchpadPath); err != nil {
		s.Fatal("Failed to clean scratchpad before the start of test: ", err)
	}
	util.CleanupKeysBeforeTest(ctx, pkcs11Util, cryptohome)

	// Prepare the scratchpad.
	f1, f2, err := pkcs11test.PrepareScratchpadAndTestFiles(ctx, r, scratchpadPath)
	if err != nil {
		s.Fatal("Failed to initialize the scratchpad space: ", err)
	}
	// Remove all keys/certs, if any at the end. i.e. Cleanup after ourselves.
	defer pkcs11test.CleanupScratchpad(ctx, r, scratchpadPath)

	// Create the various keys.
	keys, err := util.CreateKeysForTesting(ctx, r, pkcs11Util, cryptohome, scratchpadPath, util.RSAKey, s.Param().(bool))
	if err != nil {
		s.Fatal("Failed to create keys for testing: ", err)
	}
	defer func() {
		if err := util.CleanupTestingKeys(ctx, keys, pkcs11Util, cryptohome); err != nil {
			s.Error("Failed to cleanup testing keys: ", err)
		}
	}()
	// Give the cleanup 10 seconds to finish.
	shortenedCtx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Test the various keys.
	for _, k := range keys {
		// Test the various mechanisms.
		for _, m := range []pkcs11.MechanismInfo{pkcs11.SHA1RSAPKCS, pkcs11.SHA256RSAPKCS} {
			if err := pkcs11test.SignAndVerify(shortenedCtx, pkcs11Util, k, f1, f2, &m); err != nil {
				s.Error("SignAndVerify failed: ", err)
			}
		}
	}

	// Remount the cryptohome.
	if _, err := cryptohome.Unmount(ctx, util.FirstUsername); err != nil {
		s.Fatal("Failed to unmount the first user: ", err)
	}
	if err := cryptohome.MountVault(ctx, util.PasswordLabel, hwsec.NewPassAuthConfig(util.FirstUsername, util.FirstPassword), false, hwsec.NewVaultConfig()); err != nil {
		s.Fatal("Failed to re-mount the first user: ", err)
	}

	if err := cryptohome.WaitForUserToken(ctx, util.FirstUsername); err != nil {
		s.Fatal("Failed to wait for user token: ", err)
	}

	// Test the various keys again.
	for _, k := range keys {
		// Test the various mechanisms.
		for _, m := range []pkcs11.MechanismInfo{pkcs11.SHA1RSAPKCS, pkcs11.SHA256RSAPKCS} {
			if err := pkcs11test.SignAndVerify(shortenedCtx, pkcs11Util, k, f1, f2, &m); err != nil {
				s.Error("SignAndVerify after re-mount failed: ", err)
			}
		}
	}
}
