// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package familylink

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

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
		Func:         BedTimeLimit,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify the bed time limit works correctly for Family Link account",
		Contacts: []string{
			"cros-families-eng+test@google.com",
			"chromeos-sw-engprod@google.com",
			"amberhaynes@chromium.org",
		},
		// ChromeOS > Software > Family > Parental controls
		BugComponent: "b:1090157",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      10 * time.Minute,
		VarDeps:      []string{"family.unicornEmail", "family.unicornPassword"},
		Fixture:      "familyLinkUnicornPolicyLogin",
	})
}

func BedTimeLimit(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn := s.FixtValue().(familylink.HasTestConn).TestConn()

	// Make sure screen is not locked.
	s.Log("Assert the screen is not locked")
	if _, err := lockscreen.WaitState(ctx, tconn,
		func(st lockscreen.State) bool { return !st.Locked }, 30*time.Second); err != nil {
		s.Fatal("Waiting for screen to be unlocked failed: ", err)
	}

	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	now := time.Now()
	bedTimeDuration := time.Minute
	timeBeforeLocked := 30 * time.Second
	location, err := familylink.GetSystemClockLocation()
	if err != nil {
		s.Fatal("Get system clock location failed: ", err)
	}
	startAt := now.Add(timeBeforeLocked).In(location)
	endAt := startAt.Add(bedTimeDuration).In(location)

	usageLimitPolicy := familylink.CreateUsageTimeLimitPolicy()
	bedTimeLimitEntry := &policy.UsageTimeLimitValueTimeWindowLimitEntries{
		EffectiveDay:      strings.ToUpper(startAt.Weekday().String()),
		LastUpdatedMillis: strconv.FormatInt(now.Unix(), 10 /*base*/),
		EndsAt: &policy.UsageTimeLimitValueTimeWindowLimitEntriesEndsAt{
			Hour:   endAt.Hour(),
			Minute: endAt.Minute(),
		},
		StartsAt: &policy.UsageTimeLimitValueTimeWindowLimitEntriesStartsAt{
			Hour:   startAt.Hour(),
			Minute: startAt.Minute(),
		},
	}

	usageLimitPolicy.Val.TimeWindowLimit.Entries =
		append(usageLimitPolicy.Val.TimeWindowLimit.Entries, bedTimeLimitEntry)

	// Bed time duration is set by parent, which could be several hours starts at night
	// and ends in the morning. This test locks the screen after 30 seconds and unlocks it
	// after another minute. Family Link users have restrictions to prevent manipulating
	// the system clock.
	policies := []policy.Policy{
		usageLimitPolicy,
	}

	pb := policy.NewBlob()
	pb.PolicyUser = s.FixtValue().(familylink.HasPolicyUser).PolicyUser()
	pb.AddPolicies(policies)

	s.Logf("Setting a bed time limit policy with weekday=%v, startAt=%v, endAt=%v", startAt.Weekday(), startAt, endAt)
	if err := policyutil.ServeBlobAndRefresh(ctx, fdms, cr, pb); err != nil {
		s.Fatal("Failed to serve policies: ", err)
	}

	s.Log("Verifying policies were delivered to device")
	if err := policyutil.Verify(ctx, tconn, policies); err != nil {
		s.Fatal("Failed to verify policies: ", err)
	}

	ui := uiauto.New(tconn)
	lockStateUpdateTimeOut := 1 * time.Minute
	s.Log("Waiting for bed time starts in ", timeBeforeLocked)
	if _, err := lockscreen.WaitState(ctx, tconn,
		func(st lockscreen.State) bool { return st.Locked }, timeBeforeLocked+lockStateUpdateTimeOut); err != nil {
		s.Fatal("Waiting for screen to be locked failed: ", err)
	}
	if err := ui.WaitUntilExists(nodewith.Name("Time for bed").Role(role.StaticText))(ctx); err != nil {
		s.Fatal("Time for bed message is missing: ", err)
	}

	s.Log("Waiting for bed time ends in ", bedTimeDuration)
	childUser := strings.ToLower(s.RequiredVar("family.unicornEmail"))
	if err := lockscreen.WaitForPasswordField(ctx, tconn, childUser, bedTimeDuration+lockStateUpdateTimeOut); err != nil {
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
		s.Fatal(fmt.Sprintf("Waiting for screen to be unlocked failed (last status %+v): ", st), err)
	}

}
