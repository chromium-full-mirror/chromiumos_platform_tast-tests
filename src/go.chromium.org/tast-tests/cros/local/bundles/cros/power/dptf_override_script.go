// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/power/dptf"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DptfOverrideScript,
		Desc:         "Check that dptf loads correct thermal profile from override script",
		BugComponent: "b:1361410",
		Contacts: []string{
			"chromeos-platform-power@google.com", // CrOS platform power developers
			"puthik@chromium.org",                // test author
		},
		// TODO(b/210921679): re-enable once failures are fixed.
		Attr: []string{"group:mainline", "informational"},
		// Only Atlas use override script, board developed later uses unibuild.
		HardwareDeps: hwdep.D(hwdep.Model("atlas")),
	})
}

func DptfOverrideScript(ctx context.Context, s *testing.State) {
	expectedProfile, err := dptf.GetProfileFromOverrideScript(ctx)
	if err != nil {
		s.Fatal("GetProfileFromOverrideScript failed: ", err)
	}
	actualProfile, err := dptf.GetProfileFromPgrep(ctx)
	if err != nil {
		s.Fatal("GetProfileFromPgrep failed: ", err)
	}

	if expectedProfile != actualProfile {
		s.Errorf("Unexpected DPTF profile: got %q; want %q", actualProfile, expectedProfile)
	}
}
