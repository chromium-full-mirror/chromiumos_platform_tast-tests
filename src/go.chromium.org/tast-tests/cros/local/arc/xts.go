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
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	verifyAdbInstallsKey = "verifier_verify_adb_installs"
	verifierEngprodKey   = "verifier_engprod"
)

// RunXtsTests runs all tests in a specified test class from a CTS/GTS APK.
func RunXtsTests(ctx context.Context, a *ARC,
	testApkName, testApkPath, xtsPackageName, xtsTestClassName, xtsTestRunnerName string) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	// Save the original verifier settings.
	verifyAdbInstallsOriginal, err := a.Command(ctx, "settings", "get", "global", verifyAdbInstallsKey).Output(testexec.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "failed to get verifier_verify_adb_installs")
	}
	verifierEngprodOriginal, err := a.Command(ctx, "settings", "get", "global", verifierEngprodKey).Output(testexec.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "failed to get verifier_engprod")
	}

	testing.ContextLog(ctx, "Updating verifier settings")
	// Disable adb install verification so the xTS apk does not get blocked from running.
	_, err = a.Command(ctx, "settings", "put", "global", verifyAdbInstallsKey, "0").Output(testexec.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "failed to set verifier_verify_adb_installs")
	}
	defer func(ctx context.Context) {
		testing.ContextLog(ctx, "Restoring original value for verifier_verify_adb_installs")
		_, err = a.Command(ctx, "settings", "put", "global", verifyAdbInstallsKey,
			string(verifyAdbInstallsOriginal)).Output(testexec.DumpLogOnError)
		if err != nil {
			testing.ContextLog(ctx, "Failed to restore verifier_verify_adb_installs: ", err)
		}
	}(cleanupCtx)

	// Enable verifier in EngProd mode to prevent Google Play Protect UI from slowing down the test.
	_, err = a.Command(ctx, "settings", "put", "global", verifierEngprodKey, "1").Output(testexec.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "failed to set verifier_engprod")
	}
	defer func(ctx context.Context) {
		testing.ContextLog(ctx, "Restoring original value for verifier_engprod")
		_, err = a.Command(ctx, "settings", "put", "global", verifierEngprodKey,
			string(verifierEngprodOriginal)).Output(testexec.DumpLogOnError)
		if err != nil {
			testing.ContextLog(ctx, "Failed to restore verifier_engprod: ", err)
		}
	}(cleanupCtx)

	testing.ContextLog(ctx, "Installing "+testApkName)
	if err = a.Install(ctx, testApkPath, adb.InstallOptionGrantPermissions); err != nil {
		return errors.Wrap(err, "failed to install xTS apk")
	}
	defer func(ctx context.Context) {
		testing.ContextLog(ctx, "Uninstalling "+xtsPackageName)
		if err := a.Uninstall(ctx, xtsPackageName); err != nil {
			testing.ContextLog(ctx, "Failed to uninstall xTS package: ", err)
		}
	}(cleanupCtx)

	testing.ContextLog(ctx, "Running "+xtsTestClassName)
	// Example command:
	// adb shell am instrument -w -e class android.keystore.cts.KeyAttestationTest \
	// 	   android.keystore.cts/androidx.test.runner.AndroidJUnitRunner
	result, err := a.Command(ctx, "am", "instrument", "-w", "-e", "class",
		fmt.Sprintf("%s.%s", xtsPackageName, xtsTestClassName),
		fmt.Sprintf("%s/%s", xtsPackageName, xtsTestRunnerName)).Output(testexec.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "failed to run xTS tests")
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
	if regexMatch == nil {
		return errors.New("xTS tests failed: " + string(result))
	}

	testing.ContextLog(ctx, regexMatch[1]+" tests ran and passed")
	return nil
}
