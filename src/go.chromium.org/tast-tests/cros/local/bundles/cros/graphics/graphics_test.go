// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package graphics

import (
	"strings"
	gotesting "testing"

	"go.chromium.org/tast-tests/cros/local/graphics/testcheck"
	_ "go.chromium.org/tast-tests/cros/local/policyutil/fixtures" // import fixtures for policyutils in unittest
	"go.chromium.org/tast/core/testing"
)

const namePattern = "graphics.*"

func TestFixture(t *gotesting.T) {
	excludeTestPrefix := []string{
		// Exclude crostini as it needs crostini is still using precondition.
		"graphics.GLBench.crostini",
		// Exclude fast_ink as its subtest are arc-based and not support hangcheck yet.
		"graphics.FastInk",
	}

	re, err := testing.NewTestGlobRegexp(namePattern)
	if err != nil {
		t.Fatalf("Bad glob %q: %v", namePattern, err)
	}
	filter := func(t *testing.TestInstance) bool {
		for _, prefix := range excludeTestPrefix {
			if strings.HasPrefix(t.Name, prefix) {
				return false
			}
		}
		return re.MatchString(t.Name)
	}
	testcheck.CheckFixtures(t, filter, []string{"gpuWatchHangs|gpuWatchHangsEnrolled"})
	if t.Failed() {
		t.Error("If the test already has a fixture, check gpuWatchHangs is inherited in the test's fixture. Or add fixture \"graphicsChrome\" if it needs the browser, \"graphicsNoChrome\" if test should be run without ui.")
		t.Error("If the test intentionally causes alarming syslog messages, e.g. GPU hangs, consider calling graphics.DisableSysLogCheck() at the start of the test.")
	}
}
