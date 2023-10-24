// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"

	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ECVersion,
		Desc: "Verify that the EC version can be retrieved from ectool",
		Contacts: []string{
			"chromeos-faft@google.com",
			"jbettis@chromium.org",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_smoke"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

func ECVersion(ctx context.Context, s *testing.State) {
	ec := firmware.NewECTool(s.DUT(), firmware.ECToolNameMain)
	version, err := ec.Version(ctx)
	if err != nil {
		s.Fatal("Failed to determine EC version: ", err)
	}
	s.Log("EC version: ", version)
}
