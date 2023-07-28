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
		Func:         DptfOverrideCrosConfig,
		Desc:         "Check that dptf loads correct thermal profile from cros_config",
		BugComponent: "b:1361410",
		Contacts: []string{
			"chromeos-platform-power@google.com", // CrOS platform power developers
			"puthik@chromium.org",                // test author
		},
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"dptf", "unibuild"},

		// Atlas is a migrated pre-unibuild device, and uses a
		// script to override the DPTF profile instead of
		// cros_config.  New projects should use cros_config
		// only; please do not add new devices to this list.
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("atlas")),
	})
}

func DptfOverrideCrosConfig(ctx context.Context, s *testing.State) {
	expectedProfile, err := dptf.GetProfileFromCrosConfig(ctx)
	if err != nil {
		s.Fatal("GetProfileFromCrosConfig failed: ", err)
	}
	actualProfile, err := dptf.GetProfileFromPgrep(ctx)
	if err != nil {
		s.Fatal("GetProfileFromPgrep failed: ", err)
	}

	if expectedProfile != actualProfile {
		s.Errorf("Unexpected DPTF profile: got %q; want %q", actualProfile, expectedProfile)
	}
}
