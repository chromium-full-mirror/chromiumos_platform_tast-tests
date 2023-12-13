// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package reporters

import (
	"context"
	"strings"

	"go.chromium.org/tast/core/errors"
	"golang.org/x/exp/slices"
)

// CheckMiniOSBoot checks if the current boot is a MiniOS boot.
func (r *Reporter) CheckMiniOSBoot(ctx context.Context) (bool, error) {
	out, err := r.CatFile(ctx, "/proc/cmdline")
	if err != nil {
		return false, errors.Wrap(err, "failed to cat \"/proc/cmdline\"")
	}
	outSplit := strings.Split(string(out), " ")
	return slices.Contains(outSplit, "cros_minios"), nil
}
