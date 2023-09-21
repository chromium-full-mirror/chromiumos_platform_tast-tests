// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VTSKeymaster,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Runs the Android VTS module VtsHalKeymasterV3_0Target",
		Contacts:     []string{"arc-commercial@google.com", "vraheja@chromium.org"},
		// ChromeOS > Software > ARC++ > Commercial > Secret Management
		BugComponent: "b:1284082",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		// TODO(b/301629757): Switch back to |arcBooted|, when KeyMint is fully launched on ARC-T.
		Fixture: "arcBootedWithKeyMintOff",
		Timeout: 4 * time.Minute,
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_p"},
			// TODO(b/273223557): Download only one file for the current architecture.
			ExtraData: []string{
				"VtsHalKeymasterV3_0TargetTest_arm",
				"VtsHalKeymasterV3_0TargetTest_arm64",
				"VtsHalKeymasterV3_0TargetTest_x86",
				"VtsHalKeymasterV3_0TargetTest_x86_64",
			},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
			ExtraData:         []string{"VtsHalKeymasterV3_0TargetTest_rvc_bertha_x86_64"},
		}},
	})
}

func VTSKeymaster(ctx context.Context, s *testing.State) {
	a := s.FixtValue().(*arc.PreData).ARC

	testExecName, err := vtsTestExecName(ctx, a, isARCVM(s.SoftwareDeps()))
	if err != nil {
		s.Fatal("Error finding test binary name: ", err)
	}

	cleanup, err := arc.RunVtsTests(ctx, a, s.DataPath(testExecName), s.OutDir())
	if err != nil {
		s.Error("Error running test: ", err)
	}
	defer cleanup()
}

// isARCVM returns true if the test software dependencies include "android_vm".
func isARCVM(softwareDeps []string) bool {
	for _, dep := range softwareDeps {
		if dep == "android_vm" {
			return true
		}
	}
	return false
}

// vtsTestExecName returns the test binary name to be used for the current architecture.
func vtsTestExecName(ctx context.Context, a *arc.ARC, isARCVM bool) (string, error) {
	output, err := a.Command(ctx, "uname", "-m").Output(testexec.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "failed to determine container architecture")
	}

	arch := strings.TrimSpace(string(output))
	if isARCVM && arch == "x86_64" {
		return "VtsHalKeymasterV3_0TargetTest_rvc_bertha_x86_64", nil
	} else if arch == "armv7l" || arch == "armv8l" {
		return "VtsHalKeymasterV3_0TargetTest_arm", nil
	} else if arch == "aarch64" {
		return "VtsHalKeymasterV3_0TargetTest_arm64", nil
	} else if arch == "i686" {
		return "VtsHalKeymasterV3_0TargetTest_x86", nil
	} else if arch == "x86_64" {
		return "VtsHalKeymasterV3_0TargetTest_x86_64", nil
	}

	return "", errors.Errorf("no known test binary for %s architecture", arch)
}
