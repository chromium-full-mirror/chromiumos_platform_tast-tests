// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package autoupdate

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/autoupdate/util"
	"go.chromium.org/tast-tests/cros/remote/updateutil"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         BasicNToM,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Example test for updating to an older version using Nebraska and test images",
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"mpolzer@google.com", // Test owner
		},
		BugComponent: "b:1170223", // ChromeOS > Software > Commercial (Enterprise) > EngProd
		Attr:         []string{},  // Manual execution only.
		SoftwareDeps: []string{"reboot", "chrome", "auto_update_stable"},
		ServiceDeps: []string{
			"tast.cros.nebraska.Service",
			"tast.cros.autoupdate.UpdateService",
		},
		Timeout: util.TotalTestTime,
		Fixture: fixture.Autoupdate,
	})
}

func BasicNToM(ctx context.Context, s *testing.State) {
	paygen := s.FixtValue().(updateutil.WithPaygen).Paygen()
	filtered := paygen.FilterChannel("stable")
	if err := util.NToMTest(ctx, s.DUT(), s.OutDir(), s.RPCHint(), &util.Operations{}, filtered, 3 /*deltaM*/); err != nil {
		s.Error("Failed to complete the N to M update test: ", err)
	}
}
