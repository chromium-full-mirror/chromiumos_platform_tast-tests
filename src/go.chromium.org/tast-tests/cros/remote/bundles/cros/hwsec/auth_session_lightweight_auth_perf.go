// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"
	"strconv"
	"time"

	uda "chromiumos/system_api/user_data_auth_proto"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/hwsec/util"
	hwsecremote "go.chromium.org/tast-tests/cros/remote/hwsec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

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
		SoftwareDeps: []string{"tpm_clear_allowed", "reboot"},
		Vars:         []string{"hwsec.AuthSessionLightweightAuthPerf.iterations"},
	})
}

func AuthSessionLightweightAuthPerf(ctx context.Context, s *testing.State) {
	// Setup helper functions.
	r := hwsecremote.NewCmdRunner(s.DUT())
	helper, err := hwsecremote.NewHelper(r, s.DUT())
	if err != nil {
		s.Fatal("Helper creation error: ", err)
	}
	utility := helper.CryptohomeClient()
	// Reset TPM
	if err := helper.EnsureTPMAndSystemStateAreReset(ctx); err != nil {
		s.Fatal("Failed to ensure resetting TPM: ", err)
	}

	if err := utility.MountVault(ctx, util.Password1Label, hwsec.NewPassAuthConfig(util.FirstUsername, util.FirstPassword1), true /* createVault */, hwsec.NewVaultConfig()); err != nil {
		s.Fatal("Failed to create user: ", err)
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
		err := utility.WithAuthSession(ctx, util.FirstUsername, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_VERIFY_ONLY, func(authSessionID string) error {
			if _, err := utility.AuthenticateAuthFactor(ctx, authSessionID, util.Password1Label, util.FirstPassword1); err != nil {
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

	// Add a PIN factor.
	if err := utility.WithAuthSession(ctx, util.FirstUsername, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
		if _, err := utility.AuthenticateAuthFactor(ctx, authSessionID, util.Password1Label, util.FirstPassword1); err != nil {
			return errors.Wrap(err, "failed to authenticate auth factor")
		}
		if err := utility.AddPinAuthFactor(ctx, authSessionID, util.PinLabel, util.FirstPin); err != nil {
			return errors.Wrap(err, "failed to add auth factor")
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to add PIN factor: ", err)
	}

	// Run |iterations| times.
	for i := int64(0); i < iterations; i++ {
		// Simulate that there's a PIN to reset.
		if err := utility.WithAuthSession(ctx, util.FirstUsername, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
			reply, err := utility.AuthenticatePinAuthFactor(ctx, authSessionID, util.PinLabel, util.IncorrectPassword)
			if err == nil {
				return errors.New("authentication succeeds with a wrong PIN")
			}
			if reply.ErrorInfo.PrimaryAction != uda.PrimaryAction_PRIMARY_INCORRECT_AUTH {
				return errors.Errorf("authentication fails with incorrect primary action: %v, should be %v", reply.ErrorInfo.PrimaryAction, uda.PrimaryAction_PRIMARY_INCORRECT_AUTH)
			}
			return nil
		}); err != nil {
			s.Fatal("Failed to increment PIN wrong attempt: ", err)
		}

		startTs := time.Now()
		// Perform unlock for user during the session, but there is a PIN to reset.
		err = utility.WithAuthSession(ctx, util.FirstUsername, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_VERIFY_ONLY, func(authSessionID string) error {
			if _, err := utility.AuthenticateAuthFactor(ctx, authSessionID, util.Password1Label, util.FirstPassword1); err != nil {
				return errors.Wrap(err, "failed to authenticate user")
			}
			return nil
		})
		duration := time.Now().Sub(startTs)

		if err != nil {
			s.Fatal("Failed to authenticate user with password with verify intent: ", err)
		}

		value.Append(perf.Metric{
			Name:      "auth_factor_unlock_with_reset_duration",
			Unit:      "us",
			Direction: perf.SmallerIsBetter,
			Multiple:  true,
		}, float64(duration.Microseconds()))

		// Ensure PIN is reset so that state is clean for next iteration.
		// We have to poll here because the reset routine is run in the background,
		// and we don't know when it'll complete.
		if err := utility.WithAuthSession(ctx, util.FirstUsername, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
			return testing.Poll(ctx, func(ctx context.Context) error {
				_, err := utility.AuthenticatePinAuthFactor(ctx, authSessionID, util.PinLabel, util.FirstPin)
				return err
			}, &testing.PollOptions{
				Timeout:  time.Second,
				Interval: time.Millisecond * 100,
			})
		}); err != nil {
			s.Fatal("Failed to ensure PIN is reset: ", err)
		}
	}

	// Run |iterations| times to test webAuthn.
	for i := int64(0); i < iterations; i++ {
		startTs := time.Now()
		// Perform webAuthn for user during the session.
		err := utility.WithAuthSession(ctx, util.FirstUsername, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_WEBAUTHN, func(authSessionID string) error {
			if _, err := utility.AuthenticateAuthFactor(ctx, authSessionID, util.Password1Label, util.FirstPassword1); err != nil {
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
