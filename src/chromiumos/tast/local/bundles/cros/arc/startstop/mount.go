// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package startstop

import (
	"context"
	"strings"

	"chromiumos/tast/local/sysutil"
	"go.chromium.org/tast/core/testing"
)

func isARCMount(path string) bool {
	return strings.HasPrefix(path, "/opt/google/containers/android/") ||
		strings.HasPrefix(path, "/opt/google/containers/arc-") ||
		strings.HasPrefix(path, "/run/arc/")
}

// TestMount runs inside arc.StartStop.
type TestMount struct{}

// Name returns the subtest name.
func (*TestMount) Name() string { return "Mount" }

// PreStart implements Subtest.PreStart().
// It makes sure that ARC related mount points do not exist, except ones
// for Mini container.
func (*TestMount) PreStart(ctx context.Context, s *testing.State) {
	ms, err := sysutil.MountInfoForPID(sysutil.SelfPID)
	if err != nil {
		s.Fatal("Failed to get mount info: ", err)
	}

	miniContainerMounts := map[string]struct{}{
		"/opt/google/containers/android/rootfs/root":                        {},
		"/opt/google/containers/arc-obb-mounter/mountpoints/container-root": {},
		"/opt/google/containers/arc-sdcard/mountpoints/container-root":      {},
		"/run/arc/adb":             {},
		"/run/arc/adbd":            {},
		"/run/arc/debugfs/sync":    {},
		"/run/arc/debugfs/tracing": {},
		"/run/arc/media":           {},
		"/run/arc/obb":             {},
		"/run/arc/oem":             {},
		"/run/arc/sdcard":          {},
		"/run/arc/shared_mounts":   {},
	}

	for _, m := range ms {
		if !isARCMount(m.MountPath) {
			continue
		}
		if _, ok := miniContainerMounts[m.MountPath]; !ok {
			s.Error("Mountpoint leaked on login screen: ", m.MountPath)
		}
	}
}

// PostStart implements Subtest.PostStart().
func (*TestMount) PostStart(ctx context.Context, s *testing.State) {
	// Do nothing.
}

// PostStop implements Subtest.PostStop().
func (*TestMount) PostStop(ctx context.Context, s *testing.State) {
	// TODO(b:278547598): It is expected no ARC mount point,
	// however it is flaky so far and there is a random list of
	// mounts exists regardless of Chrome is completely shutdown.
}
