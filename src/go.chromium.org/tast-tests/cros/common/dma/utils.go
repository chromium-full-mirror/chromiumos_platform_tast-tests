// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dma

import (
	"fmt"
	"strings"

	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast/core/testing"
)

var dmaEnableVar = testing.RegisterVarString(
	"dma.Enable",
	"false",
	"It indicates whether dma is enabled or not",
)

func pools() (map[string]string, map[string]string) {
	var dmaPools = map[string]string{
		ui.GaiaPoolDefaultVarName: ui.GaiaDMAPoolDefaultValue(),
	}

	var regularPools = map[string]string{
		ui.GaiaPoolDefaultVarName: ui.GaiaPoolDefaultValue(),
	}

	return dmaPools, regularPools
}

func enabled() bool {
	return strings.ToLower(dmaEnableVar.Value()) == "true"
}

// CredsFromPool return proper credentials based on DMA status.
func CredsFromPool(pool string) string {
	dmaPools, regularPools := pools()

	dmaEnabled := enabled()
	if dmaEnabled {
		creds, ok := dmaPools[pool]
		if !ok {
			panic(fmt.Sprintf("Pool %q not onboard DMA yet", pool))
		}

		return creds
	}
	creds, ok := regularPools[pool]
	if !ok {
		panic(fmt.Sprintf("Pool %q not convert to global runtime variable yet", pool))
	}

	return creds
}
