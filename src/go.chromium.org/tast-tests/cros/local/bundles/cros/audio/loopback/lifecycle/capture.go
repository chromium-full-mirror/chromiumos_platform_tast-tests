// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lifecycle

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/testing"
)

func runCapture(ctx context.Context, s *testing.State, t *tester, startSec, endSec int, loopbackArgs []string) {
	t.logAction(ctx, startSec, "start capture", true)
	captureCtx, cancel := context.WithDeadline(
		ctx,
		t.t0.Add(time.Duration(endSec)*time.Second+extraTimeout),
	)
	defer cancel()

	cmd := testexec.CommandContext(
		captureCtx,
		"cras_test_client",
		fmt.Sprintf("--duration_seconds=%d", endSec-startSec),
	)
	cmd.Args = append(cmd.Args, loopbackArgs...)
	err := cmd.Run()
	t.logAction(ctx, endSec, "end capture", false)
	if err != nil {
		s.Fatal("Capture failed: ", err)
	}
}
