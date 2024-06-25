// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package familylink

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/family"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/familylink"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DailyTimeLimit,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify the daily time limit works correctly for Family Link account",
		Contacts: []string{
			"cros-families-eng+test@google.com",
			"chromeos-consumer-engprod@google.com",
		},
		// ChromeOS > Software > Family > Parental controls
		BugComponent: "b:1090157",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "gaia"},
		Timeout:      10 * time.Minute,
		VarDeps:      []string{family.UnicornAccountVarName},
		Fixture:      "familyLinkUnicornPolicyLogin",
	})
}

func DailyTimeLimit(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn := s.FixtValue().(familylink.HasTestConn).TestConn()

	// Make sure screen is not locked.
	s.Log("Assert the screen is not locked")
	if _, err := lockscreen.WaitState(ctx, tconn,
		func(st lockscreen.State) bool { return !st.Locked }, 30*time.Second); err != nil {
		s.Fatal("Waiting for screen to be unlocked failed: ", err)
	}

	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	location, err := familylink.GetSystemClockLocation()
	if err != nil {
		s.Fatal("Get system clock location failed: ", err)
	}

	now := time.Now()
	usageLimitPolicy := familylink.CreateUsageTimeLimitPolicy()

	// Set daily limit to be 1m to shorten the test.
	dailyScreenTimeLimit := time.Minute
	resetInMin := dailyScreenTimeLimit + time.Minute
	// In real life, the reset time is 6:00 am. This test sets the reset time
	// to be 1m after daily limit ends and the screen is locked (which is
	// 2m after logged in) without changing the system clock. Family Link users
	// have restrictions to prevent manipulating the system clock.
	reset := now.Add(resetInMin).In(location)
	usageLimitPolicy.Val.TimeUsageLimit.ResetAt = &policy.RefTime{
		Hour:   reset.Hour(),
		Minute: reset.Minute(),
	}

	dailyLimitEntry := &policy.RefTimeUsageLimitEntry{
		LastUpdatedMillis: strconv.FormatInt(now.Unix(), 10 /*base*/),
		UsageQuotaMins:    int(dailyScreenTimeLimit.Minutes()),
	}

	// The daily limit is applied between reset time of day N and day N+1. For example,
	// if we set 1 minute daily limit on Tuesday with the reset time to be 17:00, the
	// 1 minute daily limit will be applied between 17:00 Tuesday to 16:59 Wednesday.
	// This line calculate which date in the week should be set limit on base on `reset`
	// and set this date with `dailyLimitEntry`.
	oneDayBackward := -time.Hour * 24
	weekday := reset.Add(oneDayBackward).Weekday()
	switch weekday {
	case time.Sunday:
		usageLimitPolicy.Val.TimeUsageLimit.Sunday = &policy.UsageTimeLimitValueTimeUsageLimitSunday{
			LastUpdatedMillis: dailyLimitEntry.LastUpdatedMillis,
			UsageQuotaMins:    dailyLimitEntry.UsageQuotaMins,
		}
	case time.Monday:
		usageLimitPolicy.Val.TimeUsageLimit.Monday = &policy.UsageTimeLimitValueTimeUsageLimitMonday{
			LastUpdatedMillis: dailyLimitEntry.LastUpdatedMillis,
			UsageQuotaMins:    dailyLimitEntry.UsageQuotaMins,
		}
	case time.Tuesday:
		usageLimitPolicy.Val.TimeUsageLimit.Tuesday = &policy.UsageTimeLimitValueTimeUsageLimitTuesday{
			LastUpdatedMillis: dailyLimitEntry.LastUpdatedMillis,
			UsageQuotaMins:    dailyLimitEntry.UsageQuotaMins,
		}
	case time.Wednesday:
		usageLimitPolicy.Val.TimeUsageLimit.Wednesday = &policy.UsageTimeLimitValueTimeUsageLimitWednesday{
			LastUpdatedMillis: dailyLimitEntry.LastUpdatedMillis,
			UsageQuotaMins:    dailyLimitEntry.UsageQuotaMins,
		}
	case time.Thursday:
		usageLimitPolicy.Val.TimeUsageLimit.Thursday = &policy.UsageTimeLimitValueTimeUsageLimitThursday{
			LastUpdatedMillis: dailyLimitEntry.LastUpdatedMillis,
			UsageQuotaMins:    dailyLimitEntry.UsageQuotaMins,
		}
	case time.Friday:
		usageLimitPolicy.Val.TimeUsageLimit.Friday = &policy.RefTimeUsageLimitEntry{
			LastUpdatedMillis: dailyLimitEntry.LastUpdatedMillis,
			UsageQuotaMins:    dailyLimitEntry.UsageQuotaMins,
		}
	case time.Saturday:
		usageLimitPolicy.Val.TimeUsageLimit.Saturday = &policy.UsageTimeLimitValueTimeUsageLimitSaturday{
			LastUpdatedMillis: dailyLimitEntry.LastUpdatedMillis,
			UsageQuotaMins:    dailyLimitEntry.UsageQuotaMins,
		}
	}

	policies := []policy.Policy{
		usageLimitPolicy,
	}
	pb := policy.NewBlob()
	pb.PolicyUser = s.FixtValue().(familylink.HasPolicyUser).PolicyUser()
	pb.AddPolicies(policies)

	s.Logf("Setting a daily time limit policy with weekday=%v, reset=%v, quotaMins=%v", weekday, reset, dailyLimitEntry.UsageQuotaMins)
	if err := policyutil.ServeBlobAndRefresh(ctx, fdms, cr, pb); err != nil {
		s.Fatal("Failed to serve policies: ", err)
	}

	s.Log("Verifying policies were delivered to device")
	if err := policyutil.Verify(ctx, tconn, policies); err != nil {
		s.Fatal("Failed to verify policies: ", err)
	}

	ui := uiauto.New(tconn)
	lockStateUpdateTimeOut := 1 * time.Minute

	// The tested account on DUT might have been active before this test. The screen might be
	// locked less than `dailyScreenTimeLimit`.
	s.Log("Waiting for daily limit reaches at most in ", dailyScreenTimeLimit)
	if _, err := lockscreen.WaitState(ctx, tconn,
		func(st lockscreen.State) bool { return st.Locked }, dailyScreenTimeLimit+lockStateUpdateTimeOut); err != nil {
		s.Fatal("Waiting for screen to be locked failed: ", err)
	}
	if err := ui.WaitUntilExists(nodewith.Name("Time is up").Role(role.StaticText))(ctx); err != nil {
		s.Fatal("Time is up message is missing: ", err)
	}

	s.Log("Waiting for daily limit reset at most in ", resetInMin)
	childUser, _, err := dma.UserPassFromPool(family.UnicornAccountVarName)
	if err != nil {
		s.Fatal("Failed to get child user: ", err)
	}
	if err := lockscreen.WaitForPasswordField(ctx, tconn, childUser, resetInMin+lockStateUpdateTimeOut); err != nil {
		s.Error("Password text field did not appear in the UI: ", err)
	}

	s.Log("Trying to unlock screen")
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(ctx)
	if err := lockscreen.EnterPassword(ctx, tconn, childUser,
		s.RequiredVar("family.unicornPassword"), kb); err != nil {
		s.Fatal("Entering password failed: ", err)
	}
	if st, err := lockscreen.WaitState(ctx, tconn,
		func(st lockscreen.State) bool { return st.LoggedIn }, lockStateUpdateTimeOut); err != nil {
		s.Fatalf("Waiting for screen to be unlocked failed (last status %+v): %v", st, err)
	}

}
