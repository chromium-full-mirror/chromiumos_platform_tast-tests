// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ehide

import (
	"context"
	"strings"

	"go.chromium.org/tast-tests/cros/common/network/ehideconst"
	"go.chromium.org/tast-tests/cros/common/testexec"
)

// IsOn checks whether ehide is on.
func IsOn(ctx context.Context) (bool, error) {
	state, err := getState(ctx)
	if err != nil {
		return false, err
	}
	return state == ehideconst.EhideStateOn, nil
}

func getState(ctx context.Context) (string, error) {
	output, err := testexec.CommandContext(ctx, ehideconst.EhidePath, "state").Output(testexec.DumpLogOnError)
	if err != nil {
		return "", err
	}
	state := strings.TrimSpace(string(output))
	return state, nil
}
