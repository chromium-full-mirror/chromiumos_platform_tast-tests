// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package nearbyshare

import (
	"math"
	"math/rand"
	"strconv"

	"go.chromium.org/tast/core/testing"
)

// EnableFeatures contains optional Chrome features to be enabled with `--enable-features`.
var EnableFeatures = testing.RegisterVarString(
	"nearbyshare.EnableFeatures",
	"",
	"Chrome features to be enabled using `--enable-features`. Pass in multiple features by separating with commas i.e. Feature1,Feature2,...",
)

// RandomDeviceName appends a randomly generated integer (up to 6 digits) to the base device name to avoid conflicts
// when nearby devices in the lab may be running the same test at the same time.
func RandomDeviceName(basename string) string {
	const maxDigits = 6
	num := rand.Intn(int(math.Pow10(maxDigits) - 1))
	return basename + strconv.Itoa(num)
}
