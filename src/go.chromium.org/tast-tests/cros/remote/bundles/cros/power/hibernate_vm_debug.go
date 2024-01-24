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

type hibernateMode int

const (
	hibernateToShutdown hibernateMode = iota
	resume
)

const (
	hibernateVarCycleID = "cycleID"
)

type hibernateParams struct {
	mode hibernateMode
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         HibernateVMDebug,
		Desc:         "Hibernate tests for debugging with a virtual machine",
		BugComponent: "b:1361410",
		Contacts:     []string{"chromeos-platform-power@google.com"},
		SoftwareDeps: []string{"hibernate", "qemu"},
		Timeout:      10 * time.Minute,
		Vars:         []string{hibernate.VarEmail, hibernate.VarPassword, hibernateVarCycleID, hibernate.VarSimulateMemPressureMB},
		VarDeps:      []string{tape.ServiceAccountVar},
		Params: []testing.Param{{
			Name: "hibernate_to_shutdown",
			Val:  hibernateParams{mode: hibernateToShutdown},
		}, {
			Name: "resume",
			Val:  hibernateParams{mode: resume},
		}},
	})
}

func HibernateVMDebug(ctx context.Context, s *testing.State) {
	var err error
	mode := s.Param().(hibernateParams).mode

	params := hibernate.GetTestParams(s)

	var cycleID uint32
	if sval, ok := s.Var(hibernateVarCycleID); ok {
		val, err := strconv.ParseUint(sval, 10, 32)
		if err != nil {
			s.Fatalf("Failed to parse %s from string %s", hibernateVarCycleID, sval)
		}

		cycleID = uint32(val)
	}

	if params.UserEmail == "" || params.UserPassword == "" {
		s.Fatal("A username and password is required")
	}

	account := &tape.OwnedTestAccount{GenericAccount: tape.GenericAccount{Username: params.UserEmail, Password: params.UserPassword}}

	var ht *hibernate.Tester
	ht, err = hibernate.NewTester(ctx, s, account, hibernate.CycleMaxDuration)
	if err != nil {
		s.Fatal("Failed to create a new test: ", err)
	}
	defer func() {
		if err := ht.CleanUp(ctx); err != nil {
			s.Log("Failed to cleanup: ", err)
		}
	}()

	if cycleID > 0 {
		ht.OverrideCycleID(cycleID)
	}

	if params.MemoryPressure > 0 {
		ht.SetSimulateMemoryPressure(params.MemoryPressure)
	}

	ht.SetURLsForTabs([]string{"about:blank", "about:blank", "about:blank"})

	if mode == hibernateToShutdown {
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
