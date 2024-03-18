// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"
	"time"

	uda "go.chromium.org/chromiumos/system_api/user_data_auth_proto"
	cryptohomecommon "go.chromium.org/tast-tests/cros/common/cryptohome"
	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/common/pkcs11"
	"go.chromium.org/tast-tests/cros/common/pkcs11/pkcs11test"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/hwsec/util"
	libhwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ChapsRestart,
		Desc: "Verifies chaps works correctly after chaps restart in user session",
		Attr: []string{"group:hwsec", "hwsec_nightly", "group:chaps", "group:cryptohome"},
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

func ChapsRestart(ctx context.Context, s *testing.State) {
	checkSoftwareBacked := s.Param().(bool)

	r := libhwseclocal.NewCmdRunner()
	client := hwsec.NewCryptohomeClient(r)

	helper, err := libhwseclocal.NewHelper(r)
	if err != nil {
		s.Fatal("Failed to create hwsec helper: ", err)
	}
	cryptohome := helper.CryptohomeClient()

	daemonController := helper.DaemonController()

	pkcs11Util, err := pkcs11.NewChaps(ctx, r, cryptohome)
	if err != nil {
		s.Fatal("Failed to create PKCS#11 Utility: ", err)
	}

	const scratchpadPath = "/tmp/ChapsRestartTest"

	// Give the cleanup 15 seconds to finish.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

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
	defer pkcs11test.CleanupScratchpad(cleanupCtx, r, scratchpadPath)

	// Create the various keys.
	keys, err := util.CreateKeysForTesting(ctx, r, pkcs11Util, cryptohome, scratchpadPath, util.RSAKey, checkSoftwareBacked)
	if err != nil {
		s.Fatal("Failed to create keys for testing: ", err)
	}
	defer func() {
		if err := util.CleanupTestingKeys(cleanupCtx, keys, pkcs11Util, cryptohome); err != nil {
			s.Error("Failed to cleanup testing keys: ", err)
		}
	}()

	// Test the various keys.
	for _, k := range keys {
		// Test the various mechanisms.
		for _, m := range []pkcs11.MechanismInfo{pkcs11.SHA1RSAPKCS} {
			if err := pkcs11test.SignAndVerify(ctx, pkcs11Util, k, f1, f2, &m); err != nil {
				s.Error("SignAndVerify failed: ", err)
			}
		}
	}

	verifyPassword := func(user, password string) error {
		return client.WithAuthSession(ctx, user, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_VERIFY_ONLY, func(authSessionID string) error {
			var authReply *uda.AuthenticateAuthFactorReply
			if authReply, err = client.AuthenticateAuthFactor(ctx, authSessionID, util.PasswordLabel, password); err != nil {
				return errors.Wrap(err, "failed to authenticate user")
			}
			if err := cryptohomecommon.ExpectContainsAuthIntent(
				authReply.AuthProperties.AuthorizedFor, uda.AuthIntent_AUTH_INTENT_VERIFY_ONLY,
			); err != nil {
				return errors.Wrap(err, "unexpected AuthSession authorized intents")
			}
			return nil
		})
	}

	// Restart the chaps to simulate the case that chaps crash.
	if err := daemonController.Restart(ctx, hwsec.ChapsDaemon); err != nil {
		s.Fatal("Failed to restart chaps: ", err)
	}

	// Cryptohome should re-insert the chaps key after verify the password.
	if err := verifyPassword(util.FirstUsername, util.FirstPassword); err != nil {
		s.Fatal("Failed to authenticate first user with initial password: ", err)
	}

	if err := cryptohome.WaitForUserToken(ctx, util.FirstUsername); err != nil {
		s.Fatal("Failed to wait for user token: ", err)
	}

	// Test the various keys again.
	for _, k := range keys {
		// Test the various mechanisms.
		for _, m := range []pkcs11.MechanismInfo{pkcs11.SHA1RSAPKCS} {
			if err := pkcs11test.SignAndVerify(ctx, pkcs11Util, k, f1, f2, &m); err != nil {
				s.Error("SignAndVerify after re-mount failed: ", err)
			}
		}
	}
}
