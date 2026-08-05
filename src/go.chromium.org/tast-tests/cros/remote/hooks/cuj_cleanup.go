// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hooks

import (
	"context"
	"os"

	"go.chromium.org/tast/core/errors"
)

func init() {
	addHook(&Hook{
		Name:         "cujCleanup",
		Desc:         "Cleanup tmp cache files for CUJ tests",
		Contacts:     []string{"vincentchiang@chromium.org"},
		BugComponent: "b:974567", // ChromeOS > Software > Performance > TPS
		Impl:         &cujCleanup{},
	})
}

type cujCleanup struct{}

// SetUp cleans up the Meet experiment cache.
func (h *cujCleanup) SetUp(ctx context.Context, s *HookState) error {
	if err := os.RemoveAll("/tmp/tast/devserver/static/meet-experiments/"); err != nil {
		return errors.Wrap(err, "failed to remove Meet experiments cache")
	}
	return nil
}

// PreTest does nothing.
func (h *cujCleanup) Reset(ctx context.Context) error {
	return nil
}

// PreTest does nothing.
func (h *cujCleanup) PreTest(ctx context.Context, s *HookTestState) error {
	return nil
}

// PostTest does nothing.
func (h *cujCleanup) PostTest(ctx context.Context, s *HookTestState) error {
	return nil
}

// TearDown does nothing.
func (h *cujCleanup) TearDown(ctx context.Context, s *HookState) error {
	return nil
}
