// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package sysutil

import (
	"bytes"
	"context"
	"strings"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/lsbrelease"
	"go.chromium.org/tast/core/ssh/linuxssh"
)

// IsChromeOSFlex returns true if the test is running on ChromeOS Flex.
// Please do not use this to skip running your test entirely.
// Instead, introduce a new dependency describing the required feature:
// https://chromium.googuesource.com/chromiumos/platform/tast/+/HEAD/docs/test_dependencies.md
func IsChromeOSFlex(ctx context.Context, d *dut.DUT) (bool, error) {
	lsbContent, err := linuxssh.ReadFile(ctx, d.Conn(), "/etc/lsb-release")
	if err != nil {
		return false, err
	}
	lsb, err := lsbrelease.Parse(bytes.NewReader(lsbContent))
	if err != nil {
		return false, err
	}
	return strings.HasPrefix(lsb[lsbrelease.Board], "reven"), nil
}
