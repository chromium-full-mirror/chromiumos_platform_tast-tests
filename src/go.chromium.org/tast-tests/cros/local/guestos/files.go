// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package guestos

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// WaitForFile waits until `name` exists in `dirName` in the guest.
func WaitForFile(guest vm.Guest, dirName, name string) uiauto.Action {
	return func(ctx context.Context) error {
		return testing.Poll(ctx, func(context.Context) error {
			fileList, err := guest.GetFileList(ctx, dirName)
			if err != nil {
				return errors.Wrap(err, "couldn't get file list")
			}
			for _, file := range fileList {
				if file == name {
					return nil
				}
			}
			return errors.Errorf("file %q wasn't found", name)
		}, &testing.PollOptions{Timeout: 5 * time.Second})
	}
}

// WaitForFileGone waits until `name` doesn't exist in `dirName` in the guest.
func WaitForFileGone(guest vm.Guest, dirName, name string) uiauto.Action {
	return func(ctx context.Context) error {
		return testing.Poll(ctx, func(context.Context) error {
			fileList, err := guest.GetFileList(ctx, dirName)
			if err != nil {
				return errors.Wrap(err, "couldn't get file list")
			}
			for _, file := range fileList {
				if file == name {
					return errors.Errorf("file %q still present", name)
				}
			}
			return nil
		}, &testing.PollOptions{Timeout: 5 * time.Second})
	}
}
