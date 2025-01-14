// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/adb"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

const (
	ctsPackageName    = "android.keystore.cts"
	ctsTestClassName  = "KeyAttestationTest"
	ctsTestRunnerName = "androidx.test.runner.AndroidJUnitRunner"

	verifyAdbInstallsKey = "verifier_verify_adb_installs"
	verifierEngprodKey   = "verifier_engprod"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     CtsKeystoreAttestation,
		Desc:     "Runs the Android Keystore CTS KeyAttestationTest. These tests are not included in the full CTS run because ARC does not have secure lock screen",
		Contacts: []string{"arc-commercial@google.com", "batoon@google.com"},
		// ChromeOS > Software > ARC++ > Commercial > Tast Tests
		BugComponent: "b:1487630",
		Attr:         []string{"group:mainline", "informational"},
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 60*time.Second,
		Fixture:      "arcBooted",
		SoftwareDeps: []string{"android_vm_t", "chrome", "no_qemu"},
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
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	a := s.FixtValue().(*arc.PreData).ARC

	// Save the original verifier settings.
	verifyAdbInstallsOriginal, err := a.Command(ctx, "settings", "get", "global", verifyAdbInstallsKey).Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to get verifier_verify_adb_installs: ", err)
	}
	verifierEngprodOriginal, err := a.Command(ctx, "settings", "get", "global", verifierEngprodKey).Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to get verifier_engprod: ", err)
	}

	s.Log("Updating verifier settings")
	// Disable adb install verification so the CTS apk does not get blocked from running.
	_, err = a.Command(ctx, "settings", "put", "global", verifyAdbInstallsKey, "0").Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to set verifier_verify_adb_installs: ", err)
	}
	defer func(ctx context.Context) {
		s.Log("Restoring original value for verifier_verify_adb_installs")
		_, err = a.Command(ctx, "settings", "put", "global", verifyAdbInstallsKey,
			string(verifyAdbInstallsOriginal)).Output(testexec.DumpLogOnError)
		if err != nil {
			s.Fatal("Failed to restore verifier_verify_adb_installs: ", err)
		}
	}(cleanupCtx)

	// Enable verifier in EngProd mode to prevent Google Play Protect UI from slowing down the test.
	_, err = a.Command(ctx, "settings", "put", "global", verifierEngprodKey, "1").Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to set verifier_engprod: ", err)
	}
	defer func(ctx context.Context) {
		s.Log("Restoring original value for verifier_engprod")
		_, err = a.Command(ctx, "settings", "put", "global", verifierEngprodKey,
			string(verifierEngprodOriginal)).Output(testexec.DumpLogOnError)
		if err != nil {
			s.Fatal("Failed to restore verifier_engprod: ", err)
		}
	}(cleanupCtx)

	testApkName := s.Param().(string)
	testApkPath := s.DataPath(testApkName)
	s.Log("Installing " + testApkName)
	if err = a.Install(ctx, testApkPath, adb.InstallOptionGrantPermissions); err != nil {
		s.Fatal("Failed to install CTS apk: ", err)
	}
	defer func(ctx context.Context) {
		s.Log("Uninstalling " + ctsPackageName)
		if err := a.Uninstall(ctx, ctsPackageName); err != nil {
			s.Fatal("Failed to uninstall CTS package: ", err)
		}
	}(cleanupCtx)

	s.Log("Running " + ctsTestClassName)
	// adb shell am instrument -w -e class android.keystore.cts.KeyAttestationTest \
	// 	   android.keystore.cts/androidx.test.runner.AndroidJUnitRunner
	result, err := a.Command(ctx, "am", "instrument", "-w", "-e", "class",
		fmt.Sprintf("%s.%s", ctsPackageName, ctsTestClassName),
		fmt.Sprintf("%s/%s", ctsPackageName, ctsTestRunnerName)).Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to run CTS tests: ", err)
	}

	/* A successful result looks like this
	 *
	 *   		android.keystore.cts.KeyAttestationTest:.................
	 *   		Time: 37.628
	 *   		OK (18 tests)
	 */
	successRegexPattern := "OK \\((\\d+) tests\\)"
	re := regexp.MustCompile(successRegexPattern)
	regexMatch := re.FindStringSubmatch(string(result))
	if regexMatch != nil {
		s.Log(regexMatch[1] + " tests ran and passed")
	} else {
		s.Fatal("CTS tests failed: " + string(result))
	}
}
