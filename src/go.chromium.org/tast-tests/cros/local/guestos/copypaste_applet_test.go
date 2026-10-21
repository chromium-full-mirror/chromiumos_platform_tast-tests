// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package guestos

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
)

func TestCopyPasteApplets(t *testing.T) {
	// Run the stdlib-only CopyPasteAppletUnitTest suite with PATH's python3 so
	// CQ (cros-go_src_test) never depends on host PyGObject/Xvfb packages.
	// Full GI/Xvfb E2E tests in CopyPasteAppletGiTest run with /usr/bin/python3
	// when copypaste_applet_unittest.py is invoked directly.
	timeout := 30 * time.Second
	if deadline, ok := t.Deadline(); ok {
		rem := time.Until(deadline) - 5*time.Second
		if rem <= 0 {
			t.Fatalf("insufficient time remaining before deadline (%v)", time.Until(deadline))
		}
		if rem < timeout {
			timeout = rem
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Pass -B so running unit tests does not create __pycache__ inside the tree.
	out, err := testexec.CommandContext(
		ctx, "python3", "-B", "copypaste_applet_unittest.py", "-v", "CopyPasteAppletUnitTest",
	).CombinedOutput()
	if err != nil {
		t.Fatalf("copypaste_applet_unittest.py failed: %v\n%s", err, out)
	}
}
