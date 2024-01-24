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
	hibernateVarCycles     = "cycles"
	hibernateCyclesDefault = 1
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
		Vars:    []string{hibernateVarCycles, hibernate.VarEmail, hibernate.VarPassword, hibernate.VarSimulateMemPressureMB},
		VarDeps: []string{tape.ServiceAccountVar},
		Attr:    []string{"group:mainline", "informational"},
	})
}

func Hibernate(ctx context.Context, s *testing.State) {
	var err error

	var numCycles int
	if sval, ok := s.Var(hibernateVarCycles); ok {
		val, err := strconv.ParseInt(sval, 10, 32)
		if err != nil {
			s.Fatalf("Failed to parse %s from string %s", hibernateVarCycles, sval)
		}

		numCycles = int(val)
	} else {
		numCycles = hibernateCyclesDefault
	}

	params := hibernate.GetTestParams(s)

	var account *tape.OwnedTestAccount
	if len(params.UserEmail) > 0 && len(params.UserPassword) > 0 {
		account = &tape.OwnedTestAccount{GenericAccount: tape.GenericAccount{Username: params.UserEmail, Password: params.UserPassword}}
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

	if params.MemoryPressure > 0 {
		ht.SetSimulateMemoryPressure(params.MemoryPressure)
	}

	ht.SetURLsForTabs([]string{"about:blank", "about:blank", "about:blank"})

	for i := 1; i <= numCycles; i++ {
		if err := ht.HibernateAndResume(ctx); err != nil {
			s.Fatalf("Hibernate attempt %d failed: %v", i, err)
		}
		s.Logf("Hibernate attempt %d complete", i)
	}
}
