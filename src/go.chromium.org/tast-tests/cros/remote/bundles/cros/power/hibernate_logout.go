// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/power/hibernate"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         HibernateLogout,
		Desc:         "Verify that the 'hiberimage' DM device is torn down when the user logs out",
		BugComponent: "b:1361410",
		Contacts:     []string{"chromeos-platform-power@google.com", "mka@google.com"},
		SoftwareDeps: []string{"hibernate", "reboot", "no_qemu"},
		Timeout:      5 * time.Minute,
		VarDeps:      []string{tape.ServiceAccountVar},
		ServiceDeps:  []string{"tast.cros.platform.UpstartService"},
		Attr:         []string{"group:mainline", "informational"},
	})
}

func HibernateLogout(ctx context.Context, s *testing.State) {
	ht, err := hibernate.NewTester(ctx, s, nil, hibernate.CycleMaxDuration)
	if err != nil {
		s.Fatal("Unable to create new tester: ", err)
	}

	defer ht.CloseGRPCClient(ctx)

	// log in with a user account
	if err := ht.PreHibernateSteps(ctx, false); err != nil {
		s.Fatal("pre-hibernate steps failed: ", err)
	}

	exists, err := ht.HiberimageExists(ctx)
	if err != nil {
		s.Fatal("Failed to check if hiberimage exists: ", err)
	} else if !exists {
		s.Fatal("LV 'hiberimage' does not exist after user login")
	}

	if err := ht.Logout(ctx); err != nil {
		s.Fatal("logout failed: ", err)
	}

	exists, err = ht.HiberimageExists(ctx)
	if err != nil {
		s.Fatal("Failed to check if hiberimage exists: ", err)
	} else if exists {
		s.Fatal("LV 'hiberimage' still exists after user logged out")
	}
}
