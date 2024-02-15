// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package battery provides set of util functions for crosvm battery.
package battery

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// SetAcOnline sets the aconline state of ARCVM by crosvm command line.
func SetAcOnline(ctx context.Context, socketPath string, online bool) error {
	var onlineStr string
	if online {
		onlineStr = "1"
	} else {
		onlineStr = "0"
	}
	dump, err := testexec.CommandContext(ctx, "crosvm", "battery", "goldfish", "aconline", onlineStr, socketPath).Output(testexec.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "failed to execute crosvm command")
	}
	testing.ContextLog(ctx, string(dump))
	return nil
}
