// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package debug

import (
	"context"
	"encoding/json"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
)

// Dump fetches the audio thread debug dump.
func Dump(ctx context.Context) (*Info, error) {
	cmd := testexec.CommandContext(ctx, "cras_tests", "control", "dump_audio_debug_info", "--json")
	output, err := cmd.CombinedOutput(testexec.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "call cras_tests")
	}
	info := &Info{}
	if err := json.Unmarshal(output, info); err != nil {
		return nil, errors.Wrap(err, "unmarshal debug info json")
	}
	return info, nil
}
