// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"

	"chromiumos/tast/common/testexec"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: SHANI,
		Desc: "Run SHA-NI extension test on x86 platforms",
		Contacts: []string{
			"chromeos-faft@google.com",
			"khwon@chromium.org", // Test Author
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_ec"},
		HardwareDeps: hwdep.D(hwdep.CPUSupportsSHANI()),
		Requirements: []string{"sys-fw-0022-v01"},
	})
}

func SHANI(ctx context.Context, s *testing.State) {
	const testBin = "/usr/local/share/vboot/tests/vb2_sha256_x86_tests"
	err := testexec.CommandContext(ctx, testBin).Run(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("vb2_sha256_x86_tests failed: ", err)
	}
}
