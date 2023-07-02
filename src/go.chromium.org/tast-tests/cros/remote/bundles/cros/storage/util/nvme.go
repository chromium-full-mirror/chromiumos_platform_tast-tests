// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package util

import (
	"context"
	"regexp"
	"strconv"

	"go.chromium.org/tast/core/testing"
)

// FindSingleHexInt64NvmeRegs parses out a register hex value out of the
// identity report of the controller, e.g.
// ver       : 10400
// TODO(dlunev) should it return error instead of bool?
func FindSingleHexInt64NvmeRegs(ctx context.Context, identity, key string) (int64, bool) {
	pattern := key + `\s+:\s+(?:0x)?([0-9a-fA-F]+)`
	re := regexp.MustCompile(pattern)
	match := re.FindStringSubmatch(identity)
	if len(match) <= 1 {
		return 0, false
	}
	valueHex := match[1]
	value, err := strconv.ParseInt(valueHex, 16, 64)
	if err != nil {
		testing.ContextLogf(ctx, "Can't parse hex value %v: %q", valueHex, err)
		return 0, false
	}
	return value, true
}
