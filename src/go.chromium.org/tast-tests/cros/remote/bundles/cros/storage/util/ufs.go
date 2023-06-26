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

// FindSingleInt64ValueUfs retrieves a value of a singular keyval in UFS storage info.
// e.g.
// LUN Descriptor [Byte offset 0x17]: bProvisioningType = 0x3
// fPermanentWPEn             := 0x0
func FindSingleInt64ValueUfs(ctx context.Context, storageInfo, key string) (int64, bool) {
	pattern := key + `\s+:?=\s+0x([0-9a-fA-F]+)`
	re := regexp.MustCompile(pattern)
	match := re.FindStringSubmatch(storageInfo)
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

// FindLocalPeerInt64PairUfs retrieves values of a local/peer keyval in UFS storage info.
// e.g.
// [0x0002]TX_HSGEAR_Capability                          : local = 0x00000003, peer = 0x00000004
func FindLocalPeerInt64PairUfs(ctx context.Context, storageInfo, key string) (int64, int64, bool) {
	pattern := key + `\s+: local = 0x([0-9a-fA-F]+), peer = 0x([0-9a-fA-F]+)`
	re := regexp.MustCompile(pattern)
	match := re.FindStringSubmatch(storageInfo)
	if len(match) <= 2 {
		return 0, 0, false
	}

	localHex := match[1]
	local, err := strconv.ParseInt(localHex, 16, 64)
	if err != nil {
		testing.ContextLogf(ctx, "Can't parse local hex value %v: %q", localHex, err)
		return 0, 0, false
	}

	peerHex := match[2]
	peer, err := strconv.ParseInt(peerHex, 16, 64)
	if err != nil {
		testing.ContextLogf(ctx, "Can't parse peer hex value %v: %q", peerHex, err)
		return 0, 0, false
	}

	return local, peer, true
}
