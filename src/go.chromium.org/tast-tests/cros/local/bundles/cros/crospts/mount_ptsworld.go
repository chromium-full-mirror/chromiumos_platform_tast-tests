// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package crospts contains tast tests for PTSWorld.
package crospts

import (
	"context"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: MountPtsworld,
		Desc: "Download PTSWorld images and mount or unmount PTSWorld chroot",
		Contacts: []string{
			"cros-core-systems-perf@google.com",
			"darrenwu@google.com",
		},
		BugComponent: "b:167279", // ChromeOS > Software > Performance
		Params: []testing.Param{{
			Name:    "mount_cros_x86",
			Fixture: "mountPtsWorldForCrOSx86",
		}, {
			Name:    "unmount_cros_x86",
			Fixture: "unmountPtsWorldForCrOSx86",
		}},
	})
}

// MountPtsworld triggers fixture to mount PTSWorld and do nothing.
func MountPtsworld(ctx context.Context, s *testing.State) {
}
