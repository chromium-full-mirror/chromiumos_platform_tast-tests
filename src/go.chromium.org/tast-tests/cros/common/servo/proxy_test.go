// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package servo

import (
	"reflect"
	"testing"
)

func TestSplitHostPort(t *testing.T) {
	for _, tc := range []struct {
		input    string
		expected *connectInfo
	}{
		{"", &connectInfo{Hostname: "localhost", ServoPort: 9999}},
		{":ssh:", nil},
		{":ssh:33", &connectInfo{Hostname: "localhost", ServoPort: 9999, ServoSSHPort: 33}},
		{"rutabaga", &connectInfo{Hostname: "rutabaga", ServoPort: 9999, ServoSSHPort: 22}},
		{"rutabaga:ssh:33", &connectInfo{Hostname: "rutabaga", ServoPort: 9999, ServoSSHPort: 33}},
		{"rutabaga:1234", &connectInfo{Hostname: "rutabaga", ServoPort: 1234, ServoSSHPort: 22}},
		{"rutabaga:1234:ssh:33", &connectInfo{Hostname: "rutabaga", ServoPort: 1234, ServoSSHPort: 33}},
		{"rutabaga:localhost:1234", nil},
		{":1234", &connectInfo{Hostname: "localhost", ServoPort: 1234}},
		{":1234:ssh:", nil},
		{":1234:ssh:33", &connectInfo{Hostname: "localhost", ServoPort: 1234, ServoSSHPort: 33}},
		{"[::2]", &connectInfo{Hostname: "::2", ServoPort: 9999, ServoSSHPort: 22}},
		{"[::2]:ssh:33", &connectInfo{Hostname: "::2", ServoPort: 9999, ServoSSHPort: 33}},
		{"[::2]:1234", &connectInfo{Hostname: "::2", ServoPort: 1234, ServoSSHPort: 22}},
		{"[::2]:1234:ssh:33", &connectInfo{Hostname: "::2", ServoPort: 1234, ServoSSHPort: 33}},
		{"[::2]:localhost:1234", nil},
		{"::2", nil},
		{"::2:1234", nil},
		{"[::1]", &connectInfo{Hostname: "::1", ServoPort: 9999}},
		{"[::1]:ssh:33", &connectInfo{Hostname: "::1", ServoPort: 9999, ServoSSHPort: 33}},
		{"[::1]:1234", &connectInfo{Hostname: "::1", ServoPort: 1234}},
		{"[::1]:1234:ssh:33", &connectInfo{Hostname: "::1", ServoPort: 1234, ServoSSHPort: 33}},
		{"[::1]:localhost:1234", nil},
		{"::1", nil},
		{"::1:1234", nil},
		{"dut1-docker_servod", &connectInfo{DockerContainer: "dut1-docker_servod", ServoPort: 9999}},
		{"dut1-docker_servod:9998", &connectInfo{DockerContainer: "dut1-docker_servod", ServoPort: 9998}},
		{"dut1-docker_servod:9998::", nil},
		// Redirected ports to satlab docker servod
		{"rutabaga:1234:docker:dut1-docker_servod:tcp://localhost:5678", &connectInfo{
			Hostname: "rutabaga", ServoPort: 1234, DockerContainer: "dut1-docker_servod", DockerHost: "tcp://localhost:5678",
		}},
	} {
		info, err := splitHostPort(tc.input)
		if err != nil && tc.expected != nil {
			t.Errorf("splitHostPort(%q) returned unexpected error: %v", tc.input, err)
			continue
		}
		if err == nil && tc.expected == nil {
			t.Errorf("splitHostPort(%q) unexpectedly succeeded %+v", tc.input, info)
			continue
		}
		if !reflect.DeepEqual(info, tc.expected) {
			t.Errorf("splitHostPort(%q) got %+v, want %+v", tc.input, info, tc.expected)
		}
	}
}
