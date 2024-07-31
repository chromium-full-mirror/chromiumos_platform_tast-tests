// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         NoArcPresent,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that ARC is not present on devices with extended autoupdates",
		Contacts:     []string{"arcvm-eng@google.com", "morg@google.com", "niwa@google.com"},
		// ChromeOS > Software > ARC++
		BugComponent: "b:153255",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "extended_auto_updates"},
		Timeout:      7 * time.Minute,
	})
}

func checkFileDoesNotExist(ctx context.Context, path string) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return errors.Errorf("file %s exists", path)
}

func NoArcPresent(ctx context.Context, s *testing.State) {
	const (
		containerPath = "/opt/google/containers/android"
		vmPath        = "/opt/google/vms/android"
	)

	// Tests that the container file path does not exist and we do not have container rootfs
	if err := checkFileDoesNotExist(ctx, containerPath); err != nil {
		s.Fatal("ARC container artifacts are present on the device or something went wrong: ", err)
	}
	// Tests that the vm file path does not exist and we do not have vm rootfs
	if err := checkFileDoesNotExist(ctx, vmPath); err != nil {
		s.Fatal("ARCVM artifacts are present on the device or something went wrong: ", err)
	}
	// TODO(b/336447047): Add check to verify that arc cleanup tool is installed on the device
}
