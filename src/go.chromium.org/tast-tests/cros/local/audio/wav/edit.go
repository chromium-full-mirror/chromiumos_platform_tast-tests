// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wav

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
)

// RepeatForDuration repeats then input WAVE file for duration and writes it to output.
func RepeatForDuration(ctx context.Context, input, output string, duration time.Duration) error {
	return testexec.CommandContext(
		ctx,
		"sox",
		input,
		"--channels=2", "--rate=48000",
		output,
		"repeat", "-",
		"trim", "0", strconv.FormatFloat(duration.Seconds(), 'f', 0, 64),
	).Run(testexec.DumpLogOnError)
}
