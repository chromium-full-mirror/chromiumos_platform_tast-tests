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
	hibernateCyclesVars                   = "cycles"
	hibernateCyclesDefault                = 1
	hibernateEmailVar                     = "email"
	hibernatePasswordVar                  = "password"
	hibernateCycleIDVar                   = "cycleID"
	hibernateSimulateMemPressureMB        = "simulateMemPressureMB"
	hibernateSimulateMemPressureMBDefault = 0
)

type hibernateMode int

const (
	hibernateAndResume hibernateMode = iota
	hibernateToShutdown
	resume
)

type hibernateParams struct {
	mode hibernateMode
}

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
		Vars:    []string{hibernateCyclesVars, hibernateEmailVar, hibernatePasswordVar, hibernateCycleIDVar, hibernateSimulateMemPressureMB},
		VarDeps: []string{tape.ServiceAccountVar},
		Attr:    []string{"group:mainline", "informational"},
		Params: []testing.Param{{
			Val: hibernateParams{mode: hibernateAndResume},
		}, {
			Name: "hibernate_to_shutdown",
			Val:  hibernateParams{mode: hibernateToShutdown},
		}, {
			Name: "resume",
			Val:  hibernateParams{mode: resume},
		}},
	})
}

func Hibernate(ctx context.Context, s *testing.State) {
	var err error
	mode := s.Param().(hibernateParams).mode

	numCycles := hibernateCyclesDefault
	cycleID := uint64(0)

	if v, ok := s.Var(hibernateCyclesVars); ok {
		numCycles, err = strconv.Atoi(v)
		if err != nil {
			s.Fatalf("Failed to parse %s from string %s", hibernateCyclesVars, v)
		}
	}

	if mode != hibernateAndResume && numCycles > 1 {
		s.Fatal("You cannot specify multiple cycles with param hibernate_to_shutdown or resume")
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

	if mode == resume && (emailOverride == "" || passwordOverride == "") {
		s.Fatal("A username and password is required when doing a Resume only")
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

	simulateMemPressureMB := hibernateSimulateMemPressureMBDefault
	if v, ok := s.Var(hibernateSimulateMemPressureMB); ok {
		var err error

		simulateMemPressureMB, err = strconv.Atoi(v)
		if err != nil {
			s.Fatalf("Failed to parse %s from string %s", hibernateSimulateMemPressureMB, v)
		} else if simulateMemPressureMB < 0 || simulateMemPressureMB > 32000 {
			s.Fatalf("Invalid numeric value provided for %s : %d", hibernateSimulateMemPressureMB, simulateMemPressureMB)
		}
	}

	if simulateMemPressureMB > 0 {
		ht.SetSimulateMemoryPressure(uint32(simulateMemPressureMB))
	}

	ht.SetURLsForTabs([]string{"about:blank", "about:blank", "about:blank"})

	if mode == hibernateAndResume {
		for i := 1; i <= numCycles; i++ {
			if err := ht.HibernateAndResume(ctx); err != nil {
				s.Fatalf("Hibernate attempt %d failed: %v", i, err)
			}
			s.Logf("Hibernate attempt %d complete", i)
		}
	} else if mode == hibernateToShutdown {
		if err := ht.HibernateToShutdown(ctx); err != nil {
			s.Fatal("Hibernate to shutdown failed: ", err)
		}
		s.Log("Hibernate to shutdown complete")
	} else if mode == resume {
		if err := ht.Resume(ctx); err != nil {
			s.Fatal("Resume failed: ", err)
		}
		s.Log("Resume complete")
	}
}
