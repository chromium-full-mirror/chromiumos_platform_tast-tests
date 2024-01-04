// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VTSKeymint,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Runs the Android VTS module VtsAidlKeyMintTargetTest",
		Contacts:     []string{"arc-commercial@google.com", "yaohuali@google.com"},
		// ChromeOS > Software > ARC++ > Commercial > Tast Tests
		BugComponent: "b:1487630",
		Attr:         []string{"group:mainline", "informational"},
		VarDeps:      []string{"ui.gaiaPoolDefault"},
		SoftwareDeps: []string{"chrome"},
		// TODO(b/301629757): Switch back to |arcBooted|, when KeyMint is fully launched on ARC-T.
		Fixture: "arcBootedWithKeyMintOn",
		Timeout: chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
		Params: []testing.Param{{
			Name: "vm_x86_64",
			// TODO(b/301347001): Enable this test for ARC T+.
			ExtraSoftwareDeps: []string{"android_vm_t", "amd64"},
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
	a := s.FixtValue().(*arc.PreData).ARC
	testExecName := s.Param().(string)
	cleanup, err := arc.RunVtsTests(ctx, a, s.DataPath(testExecName), s.OutDir())
	if err != nil {
		s.Error("Error running test: ", err)
	}
	defer cleanup()
}
