// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	ctsPackageName    = "android.keystore.cts"
	ctsTestClassName  = "KeyAttestationTest"
	ctsTestRunnerName = "androidx.test.runner.AndroidJUnitRunner"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     CtsKeystoreAttestation,
		Desc:     "Runs the Android Keystore CTS KeyAttestationTest. These tests are not included in the full CTS run because ARC does not have secure lock screen",
		Contacts: []string{"arc-commercial@google.com", "batoon@google.com"},
		// ChromeOS > Software > ARC++ > Commercial > Tast Tests
		BugComponent: "b:1487630",
		Attr:         []string{"group:mainline"},
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 60*time.Second,
		Fixture:      "arcBootedWithoutUIAutomator",
		SoftwareDeps: []string{"android_vm_t", "chrome", "no_qemu"},
		HardwareDeps: hwdep.D(hwdep.MinStorage(17)), // 16GB devices may not have enough free space to install the apk.
		VarDeps:      []string{},
		Params: []testing.Param{{
			Name:              "vm_x86_64",
			ExtraSoftwareDeps: []string{"amd64"},
			ExtraData: []string{
				"CtsKeystoreTestCases_x86_64.apk",
			},
			Val: "CtsKeystoreTestCases_x86_64.apk",
		}, {
			Name:              "vm_arm64",
			ExtraSoftwareDeps: []string{"arm"},
			ExtraData: []string{
				"CtsKeystoreTestCases_arm64.apk",
			},
			Val: "CtsKeystoreTestCases_arm64.apk",
		}},
	})
}

func CtsKeystoreAttestation(ctx context.Context, s *testing.State) {
	a := s.FixtValue().(*arc.PreData).ARC
	testApkName := s.Param().(string)
	testApkPath := s.DataPath(testApkName)

	if err := arc.RunXtsTests(ctx, a, testApkName, testApkPath,
		ctsPackageName, ctsTestClassName, ctsTestRunnerName); err != nil {
		s.Fatal(ctsTestClassName+"failed: ", err)
	}
}
