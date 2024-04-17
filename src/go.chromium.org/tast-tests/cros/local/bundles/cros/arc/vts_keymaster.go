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
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VTSKeymaster,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Runs the Android VTS module VtsHalKeymasterV3_0Target",
		Contacts:     []string{"arc-commercial@google.com", "vraheja@chromium.org"},
		// ChromeOS > Software > ARC++ > Commercial > Tast Tests
		BugComponent: "b:1487630",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "arcBooted",
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
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
			Name:              "container_r",
			ExtraSoftwareDeps: []string{"android_container_r"},
			ExtraData: []string{
				"VtsHalKeymasterV3_0TargetTest_rvc_arm64",
				"VtsHalKeymasterV3_0TargetTest_rvc_bertha_x86_64",
				"VtsHalKeymasterV3_0TargetTest_arm",
				"VtsHalKeymasterV3_0TargetTest_arm64",
			},
		}, {
			Name:              "vm_r",
			ExtraSoftwareDeps: []string{"android_vm_r"},
			ExtraData: []string{
				"VtsHalKeymasterV3_0TargetTest_rvc_arm64",
				"VtsHalKeymasterV3_0TargetTest_rvc_bertha_x86_64",
				"VtsHalKeymasterV3_0TargetTest_arm",
				"VtsHalKeymasterV3_0TargetTest_arm64",
			},
		}},
	})
}

func VTSKeymaster(ctx context.Context, s *testing.State) {
	a := s.FixtValue().(*arc.PreData).ARC

	testExecName, err := vtsTestExecName(ctx, a)
	if err != nil {
		s.Fatal("Error finding test binary name: ", err)
	}

	cleanup, err := arc.RunVtsTests(ctx, a, s.DataPath(testExecName), s.OutDir())
	if err != nil {
		s.Error("Error running test: ", err)
	}
	defer cleanup()
}

// vtsTestExecName returns the test binary name to be used for the current architecture.
func vtsTestExecName(ctx context.Context, a *arc.ARC) (string, error) {
	output, err := a.Command(ctx, "uname", "-m").Output(testexec.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "failed to determine container architecture")
	}

	sdkVersion, err := arc.SDKVersion()
	if err != nil {
		return "", errors.New("failed to determine ARC SDK version")
	}

	isSDKR := sdkVersion == arc.SDKR
	arch := strings.TrimSpace(string(output))
	if isSDKR && arch == "x86_64" {
		return "VtsHalKeymasterV3_0TargetTest_rvc_bertha_x86_64", nil
	} else if isSDKR && arch == "aarch64" {
		return "VtsHalKeymasterV3_0TargetTest_rvc_arm64", nil
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
