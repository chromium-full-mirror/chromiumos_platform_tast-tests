// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package missiveutil provides helpers for missive based reporting tests.
package missiveutil

import (
	"context"
	"os"

	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
)

// ResetMissived resets reporting state clearing any existing records and keys.
func ResetMissived(ctx context.Context) error {
	if err := os.RemoveAll("/var/spool/reporting"); err != nil {
		return errors.Wrap(err, "could not remove missive dir")
	}
	if err := upstart.RestartJob(ctx, "missived"); err != nil {
		return errors.Wrap(err, "could not restart missived")
	}
	return nil
}
