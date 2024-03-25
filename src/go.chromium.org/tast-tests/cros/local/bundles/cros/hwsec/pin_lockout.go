// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"
	"math"
	"strconv"
	"time"

	uda "go.chromium.org/chromiumos/system_api/user_data_auth_proto"
	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/hwsec/util"
	cryptochrome "go.chromium.org/tast-tests/cros/local/cryptohome/chrome"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type pinLockoutTestParam struct {
	isSimulator bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         PinLockout,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that the pin lockout schedule works properly",
		Contacts: []string{
			"cryptohome-core@google.com",
			"behnoodm@chromium.org", // Test author
		},
		BugComponent: "b:1088399", // ChromeOS > Security > Cryptohome
		SoftwareDeps: []string{"pinweaver", "chrome", "gsc"},
		// If specified, instead of verifying the whole modern PIN lockout schedule,
		// the test will only verify up to index |verify_up_to_index| (0-based) of the
		// schedule.
		Vars: []string{"verify_up_to_index"},
		Params: []testing.Param{{
			// This test case takes extremely long to run, as it may verify
			// the whole PIN lockout schedule, which takes ~2 days to reach
			// the final entry. This isn't intended to be scheduled in any suite,
			// instead it can be run manually once in a while if one wants to
			// explicitly test lockout behavior on real device.
			Val: pinLockoutTestParam{
				isSimulator: false,
			},
			Timeout: 48 * time.Hour,
		}, {
			Name:              "vm",
			ExtraAttr:         []string{"group:mainline", "informational", "group:cryptohome"},
			ExtraSoftwareDeps: []string{"tpm2_simulator"},
			Val: pinLockoutTestParam{
				isSimulator: true,
			},
			Timeout: 150 * time.Second,
		}},
	})
}

// Acceptable deviation that we use to test time values. This includes delays between
// operations, and potential differences of hardware clock vs software clock.
const acceptableDeviation = time.Second * 2

type pinLockoutSchedule = []struct {
	attempts int
	lockout  time.Duration
}

// This just implements TimerHelper.WaitFor by sleeping for |duration|.
type realTimerHelper struct{}

func (helper realTimerHelper) Sleep(ctx context.Context, duration time.Duration) error {
	// GoBigSleepLint: This function sleeps for |duration| to wait for same
	// amount of time passing in the hwsec timer.
	testing.Sleep(ctx, duration)
	return nil
}

func PinLockout(ctx context.Context, s *testing.State) {
	const (
		legacyPinLockoutAttempts = 5
	)

	ctxForCleanup := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cmdRunner := hwseclocal.NewCmdRunner()
	client := hwsec.NewCryptohomeClient(cmdRunner)
	helper, err := hwseclocal.NewHelper(cmdRunner)

	var timerHelper hwsec.TimerHelper
	if s.Param().(pinLockoutTestParam).isSimulator {
		simulatorController := hwsec.NewTi50EmulatorController(cmdRunner)
		timerHelper = simulatorController.TimerHelper()
	} else {
		timerHelper = &realTimerHelper{}
	}

	if err != nil {
		s.Fatal("Failed to create hwsec local helper: ", err)
	}

	daemonController := helper.DaemonController()

	// Wait for cryptohomed to become available if needed.
	if err := daemonController.Ensure(ctx, hwsec.CryptohomeDaemon); err != nil {
		s.Fatal("Failed to ensure cryptohomed: ", err)
	}

	supportsLE, err := client.SupportsLECredentials(ctx)
	if err != nil {
		s.Fatal("Failed to get supported policies: ", err)
	} else if !supportsLE {
		s.Fatal("Device does not support PinWeaver")
	}

	// Clean up obsolete state, in case there's any.
	if err := client.UnmountAll(ctx); err != nil {
		s.Fatal("Failed to unmount vaults for preparation: ", err)
	}
	if _, err := client.RemoveVault(ctx, util.FirstUsername); err != nil {
		s.Fatal("Failed to remove old vault for preparation: ", err)
	}

	// Setup a new user with PIN while the modern pin is disabled.
	// Test the legacy PIN lockout.
	if err := cryptochrome.WithModernPinDisabled(ctx, func() error {
		if err := setupUserWithPin(ctx, ctxForCleanup, util.FirstUsername, cmdRunner, helper, true); err != nil {
			return errors.Wrap(err, "failed to set up a user with a pin auth factor while modern pin is disabled")
		}
		// The delay schedule is defined in
		// https://crsrc.org/o/src/platform2/cryptohome/auth_blocks/pin_weaver_auth_block.cc.
		legacyPinLockoutSchedule := pinLockoutSchedule{{attempts: 5, lockout: math.MaxInt64}}
		if err := checkPinLockout(ctx, client, timerHelper, util.FirstUsername, legacyPinLockoutSchedule); err != nil {
			return err
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to check legacy PIN lockout: ", err)
	}

	if err := cryptochrome.WithMigrationPin(ctx, func() error {
		_, authSessionID, err := client.StartAuthSession(ctx, util.FirstUsername, false /* ephemeral */, uda.AuthIntent_AUTH_INTENT_DECRYPT)
		if err != nil {
			return errors.Wrap(err, "failed to start auth session for PIN authentication")
		}
		defer client.InvalidateAuthSession(ctxForCleanup, authSessionID)
		// Use password auth to ensure PIN is unlocked.
		if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, util.PasswordLabel, util.FirstPassword); err != nil {
			return errors.Wrap(err, "failed to authenticate password auth factor")
		}
		// Authenticate with PIN to migrate to modern PIN.
		if _, err := client.AuthenticatePinAuthFactor(ctx, authSessionID, util.PinLabel, util.FirstPin); err != nil {
			return errors.Wrap(err, "failed to authenticate PIN auth factor")
		}

		// The delay schedule is defined in
		// https://crsrc.org/o/src/platform2/cryptohome/auth_blocks/pin_weaver_auth_block.cc.
		modernPinLockoutSchedule := pinLockoutSchedule{
			{attempts: 4, lockout: 30 * time.Second},
			{attempts: 6, lockout: time.Minute},
			{attempts: 9, lockout: 10 * time.Minute},
			{attempts: 12, lockout: 30 * time.Minute},
			{attempts: 14, lockout: 1 * time.Hour},
			{attempts: 16, lockout: 2 * time.Hour},
			{attempts: 18, lockout: 5 * time.Hour},
			{attempts: 20, lockout: 12 * time.Hour},
		}
		idxStr, ok := s.Var("verify_up_to_index")
		if ok {
			idx, err := strconv.Atoi(idxStr)
			if err != nil {
				return errors.Wrap(err, "failed to convert |verify_up_to_index| to int")
			}
			modernPinLockoutSchedule = modernPinLockoutSchedule[:idx+1]
		}
		if err := checkPinLockout(ctx, client, timerHelper, util.FirstUsername, modernPinLockoutSchedule); err != nil {
			return err
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to check modern PIN lockout: ", err)
	}
}

func checkPinLockout(ctx context.Context, client *hwsec.CryptohomeClient, timerHelper hwsec.TimerHelper, userName string, schedule pinLockoutSchedule) error {
	const wrongPin = util.SecondPin
	return client.WithAuthSession(ctx, userName, false, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
		// Ensure that before any wrong attempts, correct PIN authenticates successfully.
		if _, err := client.AuthenticatePinAuthFactor(ctx, authSessionID, util.PinLabel, util.FirstPin); err != nil {
			return errors.Wrap(err, "failed to authenticate PIN auth factor before any wrong attempts")
		}
		previousLockout := time.Duration(0)
		i := 1
		for _, entry := range schedule {
			// Ensure that the first |attempts-1| wrong attempts, PIN is locked for |previousLockout|.
			for i < entry.attempts {
				// Note: this is the i-th wrong attempt (1-based).
				if err := testSingleEntryPinLockout(ctx, client, timerHelper, authSessionID, userName, previousLockout); err != nil {
					return errors.Wrapf(err, "failed to test %d-th attempt's behavior", i)
				}
				i++
			}
			previousLockout = entry.lockout
		}
		// Test the last lockout value. The behavior shouldn't change based on
		// total number of attempts anymore after we reached the last entry.
		// Test it 2 times.
		for i := 0; i < 2; i++ {
			if err := testSingleEntryPinLockout(ctx, client, timerHelper, authSessionID, userName, previousLockout); err != nil {
				return errors.Wrapf(err, "failed to test final lockout schedule entry's behavior #%d", i)
			}
		}
		// At last, verify that PIN is unlocked now if the lockout isn't infinite.
		if previousLockout != math.MaxInt64 {
			if _, err := client.AuthenticatePinAuthFactor(ctx, authSessionID, util.PinLabel, util.FirstPin); err != nil {
				return errors.Wrap(err, "failed to authenticate PIN auth factor after testing lockout schedule")
			}
		}
		return nil
	})
}

func testSingleEntryPinLockout(ctx context.Context, client *hwsec.CryptohomeClient, timerHelper hwsec.TimerHelper, authSessionID, userName string, lockout time.Duration) error {
	// Make a wrong attempt.
	if _, err := client.AuthenticatePinAuthFactor(ctx, authSessionID, util.PinLabel, util.SecondPin); err == nil {
		return errors.New("authentication with wrong PIN unexpectedly succeeded")
	}
	// Get the lockout info.
	reply, err := client.ListAuthFactors(ctx, userName)
	if err != nil {
		return errors.Wrap(err, "failed to start auth session to check lock out info")
	}
	if err := pinHasLockout(reply, lockout); err != nil {
		return errors.Wrap(err, "failed to ensure PIN has correct lockout")
	}

	if lockout != 0 {
		// Correct PIN should fail if |lockout| isn't 0.
		if _, err := client.AuthenticatePinAuthFactor(ctx, authSessionID, util.PinLabel, util.FirstPin); err == nil {
			return errors.New("authentication with locked out PIN unexpectedly succeeded")
		}
		// If lockout is infinite, we can't test the time-based lockout.
		if lockout != math.MaxInt64 {
			// Before the lockout interval is reached, correct PIN should still fail.
			if err := timerHelper.Sleep(ctx, lockout-acceptableDeviation); err != nil {
				return errors.Wrapf(err, "failed to wait for duration %v", lockout-acceptableDeviation)
			}
			if _, err := client.AuthenticatePinAuthFactor(ctx, authSessionID, util.PinLabel, util.FirstPin); err == nil {
				return errors.New("authentication with locked out PIN unexpectedly succeeded")
			}

			// After the lockout interval is reached, the lockout info
			// should be specifying correct value. We don't test that PIN auth
			// succeeds here because we need to continue test the remaining lockout
			// schedule.
			if err := timerHelper.Sleep(ctx, acceptableDeviation*2); err != nil {
				return errors.Wrapf(err, "failed to wait for duration %v", lockout-acceptableDeviation)
			}
			reply, err := client.ListAuthFactors(ctx, userName)
			if err != nil {
				return errors.Wrap(err, "failed to list auth factors to check lock out info")
			}
			if err := pinHasLockout(reply, 0); err != nil {
				return errors.Wrap(err, "failed to ensure PIN has zero lockout")
			}
		}
	}
	return nil
}

func pinHasLockout(reply *uda.ListAuthFactorsReply, duration time.Duration) error {
	for _, entry := range reply.ConfiguredAuthFactorsWithStatus {
		if entry.AuthFactor.Type != uda.AuthFactorType_AUTH_FACTOR_TYPE_PIN {
			continue
		}
		// Check for cases when |duration| is infinite. Note that Duration is
		// Int64 and the proto field is Uint64 so we can't directly compare inifinty.
		if duration == math.MaxInt64 {
			if entry.StatusInfo.TimeAvailableIn != math.MaxUint64 {
				return errors.Errorf("unexpected lockout value: got %v, want inf", entry.StatusInfo.TimeAvailableIn)
			}
			return nil
		}
		// Otherwise, entry.StatusInfo.TimeAvailableIn should be small enough to fit Duration.
		availableIn := time.Duration(entry.StatusInfo.TimeAvailableIn) * time.Millisecond
		if availableIn <= duration && availableIn >= duration-acceptableDeviation {
			return nil
		}
		return errors.Errorf("unexpected lockout value: got %d, want %d", availableIn, duration)
	}
	return errors.New("failed to find PIN factor in user's auth factor list")
}

// setupUserWithPin sets up a user with a password and a pin.
func setupUserWithPin(ctx, ctxForCleanUp context.Context, userName string, cmdRunner *hwseclocal.CmdRunnerLocal, helper *hwseclocal.CmdHelperLocal, legacyPin bool) error {
	cryptohomeHelper := helper.CryptohomeClient()

	// Start an Auth session and get an authSessionID.
	_, authSessionID, err := cryptohomeHelper.StartAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT)
	if err != nil {
		return errors.Wrap(err, "failed to start auth session for PIN authentication")
	}
	defer cryptohomeHelper.InvalidateAuthSession(ctx, authSessionID)

	if err = cryptohomeHelper.CreatePersistentUser(ctx, authSessionID); err != nil {
		return errors.Wrap(err, "failed to create persistent user with auth session")
	}

	if _, err = cryptohomeHelper.PreparePersistentVault(ctx, authSessionID, false); err != nil {
		return errors.Wrap(err, "failed to prepare persistent user with auth session")
	}
	defer cryptohomeHelper.Unmount(ctx, userName)

	err = cryptohomeHelper.AddAuthFactor(ctx, authSessionID, util.PasswordLabel, util.FirstPassword)

	if err != nil {
		return errors.Wrap(err, "failed to add password auth factor")
	}
	// Add a PIN auth factor to the user.
	if legacyPin {
		err = cryptohomeHelper.AddPinAuthFactor(ctx, authSessionID, util.PinLabel, util.FirstPin)
	} else {
		err = cryptohomeHelper.AddModernPinAuthFactor(ctx, authSessionID, util.PinLabel, util.FirstPin)
	}

	if err != nil {
		return errors.Wrap(err, "failed to add le credential")
	}
	return nil
}
