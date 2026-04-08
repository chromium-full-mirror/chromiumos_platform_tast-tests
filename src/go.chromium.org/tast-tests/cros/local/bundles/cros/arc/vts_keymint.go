// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"reflect"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     VTSKeymint,
		Desc:     "Runs the Android VTS module VtsAidlKeyMintTargetTest",
		Contacts: []string{"arc-commercial@google.com", "yaohuali@google.com"},
		// ChromeOS > Software > ARC++ > Commercial > Tast Tests
		BugComponent: "b:1487630",
		Attr:         []string{"group:mainline"},
		VarDeps:      []string{ui.GaiaPoolDefaultVarName},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "arcBootedWithAllowAdbRoot",
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
		Params: []testing.Param{{
			Name: "vm_x86_64",
			// TODO(b/301347001): Enable this test for ARC T+.
			ExtraSoftwareDeps: []string{"android_vm_t", "amd64"},
			ExtraHardwareDeps: arc.ArcAppHwDeps,
			ExtraData: []string{
				"VtsAidlKeyMintTargetTest_x86_64",
			},
			Val: "VtsAidlKeyMintTargetTest_x86_64",
		}, {
			Name: "vm_arm64",
			// TODO(b/301347001): Enable this test for ARC T+.
			// Note: There's no "arm64", thus use "arm".
			ExtraSoftwareDeps: []string{"android_vm_t", "arm"},
			ExtraData: []string{
				"VtsAidlKeyMintTargetTest_arm64",
			},
			Val: "VtsAidlKeyMintTargetTest_arm64",
		}},
	})
}

func VTSKeymint(ctx context.Context, s *testing.State) {
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	a := s.FixtValue().(*arc.PreData).ARC

	testing.ContextLog(ctx, "Restarting adbd as root")
	if err := a.Root(ctx); err != nil {
		s.Fatal("Failed to start adb root: ", err)
	}

	// Ensure SELinux in ARC is in Permissive mode.
	res, error := a.ShellCommand(ctx, "getenforce").Output(testexec.DumpLogOnError)
	if error != nil {
		s.Fatal("Failed to get SELinux state inside ARC: ", error)
	}

	// Check if SELinux is in Enforcing mode.
	if reflect.DeepEqual(res, []byte("Enforcing\n")) {
		// Attempt to set SELinux to Permissive mode.
		if _, err := a.ShellCommand(ctx, "setenforce", "0").Output(testexec.DumpLogOnError); err != nil {
			s.Fatal("Failed to set SELinux to Permissive mode: ", err)
		}

		// Defer restoring SELinux to Enforcing mode.
		defer func() {
			if _, err := a.ShellCommand(ctx, "setenforce", "1").Output(testexec.DumpLogOnError); err != nil {
				// Handle the error appropriately (e.g., log an error message)
				s.Fatal("Failed to restore SELinux to Enforcing mode: ", err)
			}
		}()
	}

	testExecName := s.Param().(string)
	cleanup, err := arc.RunVtsTests(ctx, a, s.DataPath(testExecName), s.OutDir())
	if err != nil {
		s.Error("Error running test: ", err)
	}
	defer cleanup()
}
