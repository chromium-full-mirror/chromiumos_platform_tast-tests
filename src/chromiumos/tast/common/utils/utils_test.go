// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package utils

import (
	"testing"
)

func TestCompanionDeviceHostnames(t *testing.T) {
	testcases := []struct {
		host, suffix, result string
		shouldFail           bool
	}{
		{
			host:       "dut",
			suffix:     "-suffix",
			result:     "dut-suffix",
			shouldFail: false,
		},
		{
			host:       "dut.cros",
			suffix:     "-suffix",
			result:     "dut-suffix.cros",
			shouldFail: false,
		},
		{
			host:       "dut.domain.cros",
			suffix:     "-s",
			result:     "dut-s.domain.cros",
			shouldFail: false,
		},
		// Check failure on IP.
		{
			host:       "192.168.0.1",
			suffix:     "-suffix",
			result:     "",
			shouldFail: true,
		},
		// With port.
		{
			host:       "dut:1234",
			suffix:     "-suffix",
			result:     "dut-suffix",
			shouldFail: false,
		},
		{
			host:       "192.168.0.1:123",
			suffix:     "-suffix",
			result:     "",
			shouldFail: true,
		},
		{
			host:       "[2001:db8::1]:123",
			suffix:     "-suffix",
			result:     "",
			shouldFail: true,
		},
	}
	for _, tc := range testcases {
		ret, err := CompanionDeviceHostname(tc.host, tc.suffix)
		if tc.shouldFail {
			if err == nil {
				t.Errorf("CompanionDeviceHostname(%q, %q) succeeded, which should fail",
					tc.host, tc.suffix)
			}
		} else if err != nil {
			t.Errorf("CompanionDeviceHostname(%q, %q) failed with err = %q",
				tc.host, tc.suffix, err.Error())
		} else if ret != tc.result {
			t.Errorf("CompanionDeviceHostname(%q, %q) got %q, want %q",
				tc.host, tc.suffix, ret, tc.result)
		}
	}
}
