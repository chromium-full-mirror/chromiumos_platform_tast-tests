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
)

const (
	gtsPackageName    = "com.google.android.gts.security"
	gtsTestClassName  = "DeviceIdAttestationTest"
	gtsTestRunnerName = "androidx.test.runner.AndroidJUnitRunner"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     GtsAttestation,
		Desc:     "Runs the Android Security GTS DeviceIdAttestationTest. These tests are not included in the full GTS run",
		Contacts: []string{"arc-commercial@google.com", "batoon@google.com"},
		// ChromeOS > Software > ARC++ > Commercial > Tast Tests
		BugComponent: "b:1487630",
		Attr:         []string{"group:mainline"},
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 60*time.Second,
		Fixture:      "arcBootedWithoutUIAutomator",
		SoftwareDeps: []string{"android_vm_t", "chrome", "no_qemu"},
		VarDeps:      []string{},
		Params: []testing.Param{{
			Name:              "vm_x86_64",
			ExtraSoftwareDeps: []string{"amd64"},
			ExtraHardwareDeps: arc.ArcAppHwDeps,
			ExtraData: []string{
				"GtsGmsCoreSecurityTestApp_x86_64.apk",
			},
			Val: "GtsGmsCoreSecurityTestApp_x86_64.apk",
		}, {
			Name:              "vm_arm64",
			ExtraSoftwareDeps: []string{"arm"},
			ExtraData: []string{
				"GtsGmsCoreSecurityTestApp_arm64.apk",
			},
			Val: "GtsGmsCoreSecurityTestApp_arm64.apk",
		}},
	})
}

func GtsAttestation(ctx context.Context, s *testing.State) {
	a := s.FixtValue().(*arc.PreData).ARC
	testApkName := s.Param().(string)
	testApkPath := s.DataPath(testApkName)

	if err := arc.RunXtsTests(ctx, a, testApkName, testApkPath,
		gtsPackageName, gtsTestClassName, gtsTestRunnerName); err != nil {
		s.Fatal(gtsTestClassName+"failed: ", err)
	}
}
