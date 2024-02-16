// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package platform

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ResourcedCgroups,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that cgroups managed by resourced have the same value as before migration",
		Contacts:     []string{"chromeos-memory@google.com", "kawasin@chromium.org"},
		BugComponent: "b:167279", // ChromeOS > Platform > baseOS > Performance
		Attr:         []string{"group:mainline", "group:criticalstaging", "informational"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      30 * time.Second,
	})
}

// ResourcedCgroups checks that cgroups managed by resourced have the same value as before
// migration.
//
// TODO(b/325561298): This test will be removed when the old cgroups are removed.
func ResourcedCgroups(ctx context.Context, s *testing.State) {
	testPairs := [][2]string{
		[2]string{
			"/sys/fs/cgroup/cpuset/chrome/urgent/cpus",
			"/sys/fs/cgroup/cpuset/resourced/all/cpus",
		},
		[2]string{
			"/sys/fs/cgroup/cpuset/chrome/non-urgent/cpus",
			"/sys/fs/cgroup/cpuset/resourced/efficient/cpus",
		},
	}
	for _, pathPair := range testPairs {
		content1, err := os.ReadFile(pathPair[0])
		if err != nil {
			s.Fatal("Failed to read file: ", err)
		}
		content2, err := os.ReadFile(pathPair[1])
		if err != nil {
			s.Fatal("Failed to read file: ", err)
		}
		if string(content1) != string(content2) {
			s.Fatal("Content mismatch: ", pathPair[0], ", ", pathPair[1], " : '",
				string(content1), "', '", string(content2), "'")
		}
	}
}
