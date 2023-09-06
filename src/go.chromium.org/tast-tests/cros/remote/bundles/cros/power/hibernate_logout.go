// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"time"

	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast-tests/cros/common/tape"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/power/hibernate"
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
	})
}

func HibernateLogout(ctx context.Context, s *testing.State) {
	ht := hibernate.NewTester(ctx, s, hibernate.CycleMaxDuration)

	defer ht.CloseGRPCClient(ctx)

	// log in with a user account
	ht.PreHibernateSteps(ctx)

	if !ht.HiberimageExists(ctx) {
		s.Fatal("LV 'hiberimage' does not exist after user login")
	}

	ht.Logout(ctx)

	if ht.HiberimageExists(ctx) {
		s.Fatal("LV 'hiberimage' still exists after user logged out")
	}
}
