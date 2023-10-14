// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/power/hibernate"
	"go.chromium.org/tast/core/testing"
)

const (
	hibernateCyclesVars    = "cycles"
	hibernateCyclesDefault = 1
	hibernateEmailVar      = "email"
	hibernatePasswordVar   = "password"
	hibernateCycleIDVar    = "cycleID"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Hibernate,
		Desc:         "Verifies that system comes back after hibernation",
		BugComponent: "b:1361410",
		Contacts:     []string{"chromeos-platform-power@google.com", "vovoy@google.com"},
		SoftwareDeps: []string{"hibernate", "reboot"},
		// Allow for a larger number of cycles for stress testing. In case of a hang the
		// test will time out on one of the shorter context specific timeouts.
		Timeout: 24 * time.Hour,
		Vars:    []string{hibernateCyclesVars, hibernateEmailVar, hibernatePasswordVar, hibernateCycleIDVar},
		VarDeps: []string{tape.ServiceAccountVar},
		Attr:    []string{"group:mainline", "informational"},
	})
}

func Hibernate(ctx context.Context, s *testing.State) {
	var err error
	numCycles := hibernateCyclesDefault
	cycleID := uint64(0)

	if v, ok := s.Var(hibernateCyclesVars); ok {
		numCycles, err = strconv.Atoi(v)
		if err != nil {
			s.Fatalf("Failed to parse %s from string %s", hibernateCyclesVars, v)
		}
	}

	if v, ok := s.Var(hibernateCycleIDVar); ok {
		cycleID, err = strconv.ParseUint(v, 10, 32)
		if err != nil {
			s.Fatalf("Failed to parse %s from string %s", hibernateCycleIDVar, v)
		}
	}

	var emailOverride string
	var passwordOverride string
	var ok bool
	if emailOverride, ok = s.Var(hibernateEmailVar); !ok {
		emailOverride = ""
	}

	if passwordOverride, ok = s.Var(hibernatePasswordVar); !ok {
		passwordOverride = ""
	}

	var account *tape.OwnedTestAccount
	if len(emailOverride) > 0 && len(passwordOverride) > 0 {
		account = &tape.OwnedTestAccount{GenericAccount: tape.GenericAccount{Username: emailOverride, Password: passwordOverride}}
	}

	var ht *hibernate.Tester
	ht, err = hibernate.NewTester(ctx, s, account, time.Duration(numCycles)*hibernate.CycleMaxDuration)
	if err != nil {
		s.Fatal("Failed to create a new test: ", err)
	}
	defer func() {
		if err := ht.CleanUp(ctx); err != nil {
			s.Log("Failed to cleanup: ", err)
		}
	}()

	if cycleID > 0 {
		ht.OverrideCycleID(uint32(cycleID))
	}

	ht.SetURLsForTabs([]string{"about:blank", "about:blank", "about:blank"})

	for i := 1; i <= numCycles; i++ {
		if err := ht.HibernateAndResume(ctx); err != nil {
			s.Fatalf("Hibernate attempt %d failed: %v", i, err)
		}
		s.Logf("Hibernate attempt %d complete", i)
	}
}
