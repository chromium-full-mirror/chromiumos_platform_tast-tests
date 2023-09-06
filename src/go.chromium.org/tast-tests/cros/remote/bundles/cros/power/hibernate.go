// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast-tests/cros/common/tape"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/power/hibernate"
)

const (
	hibernateCyclesVars    = "cycles"
	hibernateCyclesDefault = 1
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Hibernate,
		Desc:         "Verifies that system comes back after hibernation",
		BugComponent: "b:1361410",
		Contacts:     []string{"chromeos-platform-power@google.com", "vovoy@google.com"},
		SoftwareDeps: []string{"hibernate", "reboot", "no_qemu"},
		// Allow for a larger number of cycles for stress testing. In case of a hang the
		// test will time out on one of the shorter context specific timeouts.
		Timeout: 24 * time.Hour,
		Vars:    []string{hibernateCyclesVars},
		VarDeps: []string{tape.ServiceAccountVar},
	})
}

func Hibernate(ctx context.Context, s *testing.State) {
	numCycles := hibernateCyclesDefault

	if v, ok := s.Var(hibernateCyclesVars); ok {
		var err error

		numCycles, err = strconv.Atoi(v)
		if err != nil {
			s.Fatalf("Failed to parse %s from string %s", hibernateCyclesVars, v)
		}
	}

	ht := hibernate.NewTester(ctx, s, time.Duration(numCycles)*hibernate.CycleMaxDuration)

	ht.SetURLsForTabs([]string{"about:blank", "about:blank", "about:blank"})

	for i := 1; i <= numCycles; i++ {
		ht.HibernateAndResume(ctx)
		s.Logf("Hibernate attempt %d complete", i)
	}
}
