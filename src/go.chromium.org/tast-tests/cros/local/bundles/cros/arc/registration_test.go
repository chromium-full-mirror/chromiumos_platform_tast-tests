// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	gotesting "testing"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/testcheck"
)

const pattern = "arc.*"

func TestTimeout(t *gotesting.T) {
	const minTestBodyTime = 30 * time.Second
	minTimeout := chrome.LoginTimeout + arc.BootTimeout + minTestBodyTime

	re, err := testing.NewTestGlobRegexp(pattern)
	if err != nil {
		t.Fatalf("Bad glob %q: %v", pattern, err)
	}
	filter := func(t *testing.TestInstance) bool {
		// Only arc.* tests are interesting.
		if !re.MatchString(t.Name) {
			return false
		}
		// If the test has an ARC precondition or an ARC fixture, like arc.Booted(), it is not
		// necessary to extend the timeout, so skip them.
		if t.Pre == arc.Booted() ||
			t.Fixture == "arcBooted" ||
			t.Fixture == "arcBootedInTabletMode" ||
			t.Fixture == "arcBootedWithPvSchedEnabled" ||
			t.Fixture == "arcBootedWithoutUIAutomator" ||
			t.Fixture == "arcBootedWithFieldTrialConfigOff" ||
			t.Fixture == "arcBootedWithFieldTrialConfigOn" {
			return false
		}
		return true
	}
	testcheck.Timeout(t, filter, minTimeout)
}

func TestSoftwareDeps(t *gotesting.T) {
	testcheck.SoftwareDeps(t, testcheck.Glob(t, pattern), []string{"chrome", "android_vm|android_vm_t|android_vm_r|android_container|android_container_r|android_p|android_r|arc"})
}
