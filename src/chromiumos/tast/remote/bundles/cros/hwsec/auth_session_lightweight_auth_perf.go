// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"
	"strconv"
	"time"

	uda "chromiumos/system_api/user_data_auth_proto"
	"chromiumos/tast/common/hwsec"
	"chromiumos/tast/common/perf"
	"chromiumos/tast/errors"
	"chromiumos/tast/remote/bundles/cros/hwsec/util"
	hwsecremote "chromiumos/tast/remote/hwsec"
	"chromiumos/tast/testing"
)

// authSessionLightweightAuthPerfParams contains the test parameters that specifies the type of storage.
type authSessionLightweightAuthPerfParams struct {
	// Specifies whether to use user secret stash.
	useUserSecretStash bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func: AuthSessionLightweightAuthPerf,
		Desc: "Performance for Verify and WebAuthn operation wut AuthSession",
		Contacts: []string{
			"cros-hwsec@google.com",
			"dlunev@chromium.org", // Test author
		},
		BugComponent: "b:1188704",
		Attr:         []string{"hwsec_destructive_crosbolt_perbuild", "group:hwsec_destructive_crosbolt"},
		SoftwareDeps: []string{"tpm", "reboot"},
		Vars:         []string{"hwsec.AuthSessionLightweightAuthPerf.iterations"},
		Params: []testing.Param{{
			Name: "uss",
			Val: authSessionLightweightAuthPerfParams{
				useUserSecretStash: true,
			},
		}, {
			Name: "vk",
			Val: authSessionLightweightAuthPerfParams{
				useUserSecretStash: false,
			},
		}},
	})
}

func AuthSessionLightweightAuthPerf(ctx context.Context, s *testing.State) {
	userParam := s.Param().(authSessionLightweightAuthPerfParams)

	// Setup helper functions.
	r := hwsecremote.NewCmdRunner(s.DUT())
	helper, err := hwsecremote.NewHelper(r, s.DUT())
	if err != nil {
		s.Fatal("Helper creation error: ", err)
	}
	utility := helper.CryptohomeClient()
	utility.SetMountAPIParam(&hwsec.CryptohomeMountAPIParam{MountAPI: hwsec.AuthFactorMountAPI})

	// Reset TPM
	if err := helper.EnsureTPMAndSystemStateAreReset(ctx); err != nil {
		s.Fatal("Failed to ensure resetting TPM: ", err)
	}
	if err := utility.MountVault(ctx, util.Password1Label, hwsec.NewPassAuthConfig(util.FirstUsername, util.FirstPassword1), true /* createVault */, hwsec.NewVaultConfig()); err != nil {
		s.Fatal("Failed to create user: ", err)
	}

	if userParam.useUserSecretStash {
		// Enable UserSecretStash.
		cleanupUSSExperiment, err := helper.EnableUserSecretStash(ctx)
		if err != nil {
			s.Fatal("Failed to enable the UserSecretStash experiment: ", err)
		}
		defer cleanupUSSExperiment(ctx)
	} else {
		// Disable UserSecretStash to use VaultKeyset.
		cleanupUSSDisable, err := helper.DisableUserSecretStash(ctx)
		if err != nil {
			s.Fatal("Failed to disable the UserSecretStash experiment: ", err)
		}
		defer cleanupUSSDisable(ctx)
	}

	// Cleanup upon finishing
	defer func() {
		if _, err := utility.Unmount(ctx, util.FirstUsername); err != nil {
			s.Error("Failed to unmount vault: ", err)
		}
		if _, err := utility.RemoveVault(ctx, util.FirstUsername); err != nil {
			s.Fatal("Failed to remove vault: ", err)
		}
	}()

	// Get iterations count from the variable or default it.
	iterations := int64(50)
	if val, ok := s.Var("hwsec.AuthSessionLightweightAuthPerf.iterations"); ok {
		var err error
		iterations, err = strconv.ParseInt(val, 10, 64)
		if err != nil {
			s.Fatal("Unparsable iterations variable: ", err)
		}
	}

	value := perf.NewValues()

	// Run |iterations| times.
	for i := int64(0); i < iterations; i++ {
		startTs := time.Now()
		// Perform unlock for user during the session.
		err = utility.WithAuthSession(ctx, util.FirstUsername, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_VERIFY_ONLY, func(authSessionID string) error {
			if _, err = utility.AuthenticateAuthFactor(ctx, authSessionID, util.Password1Label, util.FirstPassword1); err != nil {
				return errors.Wrap(err, "failed to authenticate user")
			}
			return nil
		})
		duration := time.Now().Sub(startTs)

		if err != nil {
			s.Fatal("Failed to authenticate user with password with verify intent: ", err)
		}

		value.Append(perf.Metric{
			Name:      "auth_factor_unlock_duration",
			Unit:      "us",
			Direction: perf.SmallerIsBetter,
			Multiple:  true,
		}, float64(duration.Microseconds()))
	}

	// Run |iterations| times to test webAuthn.
	for i := int64(0); i < iterations; i++ {
		startTs := time.Now()
		// Perform webAuthn for user during the session.
		err = utility.WithAuthSession(ctx, util.FirstUsername, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_WEBAUTHN, func(authSessionID string) error {
			if _, err = utility.AuthenticateAuthFactor(ctx, authSessionID, util.Password1Label, util.FirstPassword1); err != nil {
				return errors.Wrap(err, "failed to authenticate user")
			}
			return nil
		})

		duration := time.Now().Sub(startTs)
		if err != nil {
			s.Fatal("Call to unlock webauthn secret resulted in an error: ", err)
		}

		value.Append(perf.Metric{
			Name:      "auth_factor_unlock_webauthn_secret_duration",
			Unit:      "us",
			Direction: perf.SmallerIsBetter,
			Multiple:  true,
		}, float64(duration.Microseconds()))
	}

	if err := value.Save(s.OutDir()); err != nil {
		s.Fatal("Failed to save perf-results: ", err)
	}
}
