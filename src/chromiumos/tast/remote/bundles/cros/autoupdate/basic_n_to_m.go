// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package autoupdate

import (
	"context"

	"chromiumos/tast/common/fixture"
	"chromiumos/tast/remote/bundles/cros/autoupdate/util"
	"chromiumos/tast/remote/updateutil"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         BasicNToM,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Example test for updating to an older version using Nebraska and test images",
		Contacts: []string{
			"gabormagda@google.com", // Test author
			"chromeos-commercial-remote-management@google.com",
		},
		BugComponent: "b:1170223",
		Attr:         []string{}, // Manual execution only.
		SoftwareDeps: []string{"reboot", "chrome", "auto_update_stable"},
		ServiceDeps: []string{
			"tast.cros.autoupdate.NebraskaService",
			"tast.cros.autoupdate.UpdateService",
		},
		Timeout: util.TotalTestTime,
		Fixture: fixture.Autoupdate,
	})
}

func BasicNToM(ctx context.Context, s *testing.State) {
	paygen := s.FixtValue().(updateutil.WithPaygen).Paygen()
	filtered := paygen.FilterChannel("stable").FilterDeltaTypes([]string{"OMAHA", "MILESTONE"})
	if err := util.NToMTest(ctx, s.DUT(), s.OutDir(), s.RPCHint(), &util.Operations{}, filtered, 3 /*deltaM*/); err != nil {
		s.Error("Failed to complete the N to M update test: ", err)
	}
}
