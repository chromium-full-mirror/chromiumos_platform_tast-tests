// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package webrtc

import (
	gotesting "testing"

	"go.chromium.org/tast-tests/cros/local/graphics/testcheck"
	_ "go.chromium.org/tast-tests/cros/local/media/pre"
	tastcheck "go.chromium.org/tast/core/testing/testcheck"
)

const namePattern = "webrtc.*"

func TestFixture(t *gotesting.T) {
	testcheck.CheckFixtures(t, tastcheck.Glob(t, namePattern), []string{"gpuWatchHangs|gpuWatchHangsEnrolled"})
	if t.Failed() {
		t.Error("If the test already has a fixture, check gpuWatchHangs is inherited in the test's fixture. Or add fixture \"chromeVideo\".")
		t.Error("If the test intentionally causes alarming syslog messages, e.g. GPU hangs, consider calling graphics.DisableSysLogCheck at the start of the test.")
	}
}
