// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package hooks contains code for support adding custom hooks to root fixture.
package hooks

import (
	"context"
	"fmt"

	"go.chromium.org/tast/core/testing"
)

func init() {
	addHook(&Hook{
		Name:         "debugInfoHook",
		Desc:         "Put various debugging information to test artifacts",
		Contacts:     []string{"chromeos-kernel-health@google.com", "shraash@google.com"},
		BugComponent: "b:167278", // ChromeOS > Platform > baseOS > Kernel
		Impl:         &debugInfoHook{},
	})
}

const (
	// File to append start and end of tests (dmesg).
	kmsgFile = "/dev/kmsg"
)

type debugInfoHook struct {
	shouldRun bool
}

// SetUp will check if /dev/kmsg is accessible, and if not it will disable the hook.
func (h *debugInfoHook) SetUp(ctx context.Context, s *HookState) error {
	h.shouldRun = false
	if s.DUT() == nil || s.DUT().Conn() == nil {
		return nil
	}
	if err := s.DUT().Conn().CommandContext(ctx, "ls", kmsgFile).Run(); err != nil {
		testing.ContextLogf(ctx, "Unable to access %s: %v", kmsgFile, err)
		return nil
	}
	h.shouldRun = true
	return nil
}

// Reset will not do anything for this hook.
func (*debugInfoHook) Reset(context.Context) error {
	return nil
}

// PreTest will append the test name to DUT's dmesg at the start of each test.
func (h *debugInfoHook) PreTest(ctx context.Context, s *HookTestState) error {
	if !h.shouldRun {
		return nil
	}
	return appendTestNameToDmesg(ctx, s, "start")
}

// PostTest will append the test name to DUT's dmesg at the end of each test.
func (h *debugInfoHook) PostTest(ctx context.Context, s *HookTestState) error {
	if !h.shouldRun {
		return nil
	}
	return appendTestNameToDmesg(ctx, s, "end")
}

// TearDown will not do anything.
func (*debugInfoHook) TearDown(context.Context, *HookState) error {
	return nil
}

func appendTestNameToDmesg(ctx context.Context, s *HookTestState, testStage string) error {
	if s.DUT() == nil || s.DUT().Conn() == nil {
		return nil
	}
	cmd := fmt.Sprintf("echo tast: %s ======== %s >> %s", s.TestName(), testStage, kmsgFile)
	if err := s.DUT().Conn().CommandContext(ctx, "sh", "-c", cmd).Run(); err != nil {
		testing.ContextLog(ctx, "Failed to append test name to DUT's dmesg: ", err)
	}
	return nil
}
