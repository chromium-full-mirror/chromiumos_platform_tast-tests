// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package sched

import (
	"context"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast/core/testing"
)

const (
	cpuCgroupPath = "/sys/fs/cgroup/cpu"
)

var allowPaths = map[string]struct{}{
	// /chrome_renderers/foreground is allowed due to a historical reason.
	// TODO(b/333974157): Remove after schedqos is launched.
	"/sys/fs/cgroup/cpu/chrome_renderers/foreground": struct{}{},
	// /chrome_renderers/background is allowed due to a historical reason.
	// TODO(b/333974157): Remove after schedqos is launched.
	"/sys/fs/cgroup/cpu/chrome_renderers/background": struct{}{},
	// ARC containers uses nested cpu cgroup (e.g. grunt and hana) b/391793867.
	"/sys/fs/cgroup/cpu/session_manager_containers/android": struct{}{},
	// ARC containers uses nested cpu cgroup (e.g. grunt and hana) b/391793867.
	"/sys/fs/cgroup/cpu/session_manager_containers/android/background": struct{}{},
	// ARC containers uses nested cpu cgroup (e.g. grunt and hana) b/391793867.
	"/sys/fs/cgroup/cpu/session_manager_containers/android/foreground": struct{}{},
	// ARC containers uses nested cpu cgroup (e.g. grunt and hana) b/391793867.
	"/sys/fs/cgroup/cpu/session_manager_containers/android/rt": struct{}{},
	// ARC containers uses nested cpu cgroup (e.g. grunt and hana) b/391793867.
	"/sys/fs/cgroup/cpu/session_manager_containers/android/top-app": struct{}{},
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         NoNestedCPUCgroups,
		Desc:         "Ensures that no nested cpu cgroups are created. Nested cpu cgroup has overhead on each scheduling",
		Contacts:     []string{"baseos-perf@google.com", "kawasin@google.com"},
		BugComponent: "b:167279", // ChromeOS > Platform > baseOS > Performance
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"chrome"},
		// Check the cpu cgroups structure at the timing when user login.
		Fixture: fixture.ChromeLoggedIn,
		Timeout: 30 * time.Second,
	})
}

func NoNestedCPUCgroups(ctx context.Context, s *testing.State) {
	rootLevel := strings.Count(cpuCgroupPath, "/")

	err := filepath.WalkDir(cpuCgroupPath, func(path string, d fs.DirEntry, err error) error {
		if !d.IsDir() {
			// cgroups are directories, so nothing to see here.
			return nil
		}
		if err != nil {
			return err
		}
		s.Log("Checking ", path)
		level := strings.Count(path, "/")
		if level > rootLevel+1 {
			if _, ok := allowPaths[path]; ok {
				s.Log("Exempted nested cpu cgroup: ", path)
			} else {
				s.Error("Nested cpu cgroup is not recommended because it adds overhead on scheduling. Put the nested cgroup to allowPaths of sched.NoNestedCPUCgroups only if you can accept the overhead: ", path)
			}
		}
		return nil
	})
	if err != nil {
		s.Fatal("Failed to walk through cpu cgroups: ", err)
	}
}
