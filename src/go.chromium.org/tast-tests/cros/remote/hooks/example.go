// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package hooks contains code for support adding custom hooks to root fixture.
package hooks

import (
	"context"

	"go.chromium.org/tast/core/testing"
)

func init() {
	addHook(&Hook{
		Name:         "exampleHook",
		Desc:         "Demonstrate how to use hook",
		Contacts:     []string{"tast-owner@google.com", "seewaifu@google.com"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		Impl:         &testhook{},
	})
}

var shouldRun = testing.RegisterVarString(
	"hooks.example.shouldrun",
	"",
	"A variable to decide whether example hook should run",
)

// testhook is for testing only
// TODO: remove this after testing is done.
type testhook struct {
	shouldRun bool
}

// SetUp will be called during root fixture Setup.
func (h *testhook) SetUp(ctx context.Context, s *HookState) error {
	h.shouldRun = shouldRun.Value() == "true"
	if !h.shouldRun {
		return nil
	}
	testing.ContextLog(ctx, "testhook Setup")
	return nil
}

// Reset will be called during root fixture Reset.
func (h *testhook) Reset(ctx context.Context) error {
	if !h.shouldRun {
		return nil
	}
	testing.ContextLog(ctx, "testhook Reset")
	return nil
}

// PreTest will be called during root fixture PreTest.
func (h *testhook) PreTest(ctx context.Context, s *HookTestState) error {
	if !h.shouldRun {
		return nil
	}
	testing.ContextLog(ctx, "testhook PreTest: ", s.TestName())
	return nil
}

// PostTest will be called during root fixture PostTest.
func (h *testhook) PostTest(ctx context.Context, s *HookTestState) error {
	if !h.shouldRun {
		return nil
	}
	testing.ContextLog(ctx, "testhook PostTest: ", s.TestName())
	return nil
}

// TearDown will be called during root fixture TearDown.
func (h *testhook) TearDown(ctx context.Context, s *HookState) error {
	if !h.shouldRun {
		return nil
	}
	testing.ContextLog(ctx, "testhook TearDown")
	return nil
}
