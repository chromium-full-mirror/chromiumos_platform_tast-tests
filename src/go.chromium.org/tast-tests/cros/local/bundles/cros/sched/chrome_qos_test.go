// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package sched

import "testing"

func TestParseCgroup(t *testing.T) {
	const cgroupV1 = `12:cpuset:/resourced/all
11:cpu:/normal
10:freezer:/
0::/
`
	for _, tc := range []struct {
		name       string
		content    string
		controller string
		want       string
		wantErr    bool
	}{
		{"cpu", cgroupV1, "cpu", "/normal", false},
		{"cpuset", cgroupV1, "cpuset", "/resourced/all", false},
		{"missing controller", cgroupV1, "memory", "", true},
		{"cgroup v2 only", "0::/\n", "cpu", "", true},
		{"path with colon", "11:cpu:/foo:bar\n", "cpu", "/foo:bar", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseCgroup(tc.content, tc.controller)
			if tc.wantErr {
				if err == nil {
					t.Errorf("parseCgroup(%q, %q) = %q; want an error", tc.content, tc.controller, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseCgroup(%q, %q) failed: %v", tc.content, tc.controller, err)
			}
			if got != tc.want {
				t.Errorf("parseCgroup(%q, %q) = %q; want %q", tc.content, tc.controller, got, tc.want)
			}
		})
	}
}
