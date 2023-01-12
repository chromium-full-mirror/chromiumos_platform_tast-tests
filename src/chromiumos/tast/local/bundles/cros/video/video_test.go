// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package video

import (
	gotesting "testing"

	"chromiumos/tast/local/graphics/testcheck"
	_ "chromiumos/tast/local/media/pre"
	tastcheck "chromiumos/tast/testing/testcheck"
)

const namePattern = "video.*"

func TestFixture(t *gotesting.T) {
	testcheck.CheckFixtures(t, tastcheck.Glob(t, namePattern), []string{"gpuWatchHangs|gpuWatchHangsEnrolled"})
	if t.Failed() {
		t.Error("If the test already has a fixture, check gpuWatchHangs is inherited in the test's fixture. Or add fixture \"chromeVideo\".")
		t.Error("If the test intentionally produces GPU hangs, consider calling graphics.DisableHangCheck() at the start of the test.")
	}
}
