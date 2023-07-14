// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifiutil

import (
	"golang.org/x/exp/constraints"

	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
)

// CommonAPOptions generates a set of options with common settings on protocol
// and channel appended with given extra options.
// This function is useful for the tests which don't quite care about the
// protocol used but need to set some other options like SSID or security.
func CommonAPOptions(extraOps ...hostapd.Option) []hostapd.Option {
	// Common case: 80211n, 5GHz channel, 40 MHz width.
	commonOps := []hostapd.Option{
		hostapd.Mode(hostapd.Mode80211nMixed),
		hostapd.Channel(48),
		hostapd.HTCaps(hostapd.HTCapHT40),
	}
	// Append extra options.
	return append(commonOps, extraOps...)
}

// Average computes the arithmetic mean of an array of numbers. An empty array
// will return 0.
func Average[T constraints.Float | constraints.Integer](data []T) float64 {
	if len(data) == 0 {
		return 0
	}
	var sum T = 0
	for _, v := range data {
		sum += v
	}
	return float64(sum) / float64(len(data))
}
