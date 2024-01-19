// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package setup

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// ChargeLimitEnabled indicates if Charge Limit is enabled for powerd via its
// configs.
func ChargeLimitEnabled(ctx context.Context) bool {
	return testexec.CommandContext(ctx, "check_powerd_config", "--charge_limit_enabled").Run() == nil
}

// ChargeControlV2Support indicates if EC supports chargecontrol V2 by checking
// exit status of "ectool chargecontrol".
func ChargeControlV2Support(ctx context.Context) bool {
	return testexec.CommandContext(ctx, "ectool", "chargecontrol").Run() == nil
}

const chargeLimitEnabledPrefPath = "/var/lib/power_manager/charge_limit_enabled"

func waitForChargeLimit(ctx context.Context) error {
	var state string
	if ChargeLimitEnabled(ctx) {
		state = "on"
	} else {
		state = "off"
	}
	// Wait for ectool to report the same state that the powerd config expects.
	return testing.Poll(ctx, func(ctx context.Context) error {
		stdout, err := testexec.CommandContext(ctx, "ectool", "chargecontrol").Output()
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to run 'ectool chargecontrol'"))
		}
		regex := regexp.MustCompile(`Battery sustainer = ([a-z]+)`)
		times := regex.FindStringSubmatch(string(stdout))
		if len(times) == 0 {
			return errors.New("Output of 'ectool chargecontrol' did not match regex")
		}
		if times[1] != state {
			return errors.Errorf("Battery sustainer is not change to state %s yet", state)
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  30 * time.Second,
		Interval: time.Second,
	})
}

// StopChargeLimit changes the charge_limit_enabled pref for powerd to 0
// (disabled) via TmpPrefs. It will then restart powerd and wait for Charge
// Limit to take effect.
func StopChargeLimit(ctx context.Context, prefs *TmpPrefs) (CleanupCallback, error) {
	if err := prefs.SetTmpPref(ctx, "charge_limit_enabled", "0"); err != nil {
		testing.ContextLog(ctx, "Failed to set charge_limit_enabled pref to 0: ", err)
		return nil, err
	}
	if err := prefs.CommitTmpPrefs(ctx, waitForChargeLimit); err != nil {
		testing.ContextLog(ctx, "Failed to commit pref change for charge_limit_enabled: ", err)
		return nil, err
	}
	// There's no need for a CleanupCallback, since TmpPrefs will wipe any
	// changes we made in its CleanupCallback.
	return nil, nil
}
