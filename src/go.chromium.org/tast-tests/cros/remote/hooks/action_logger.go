// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package hooks contains code for support adding custom hooks to root fixture.
package hooks

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	alcommon "go.chromium.org/tast-tests/cros/common/actionlogger"
	pb "go.chromium.org/tast-tests/cros/services/cros/actionlogger"
	"go.chromium.org/tast/core/rpc"
)

func init() {
	addHook(&Hook{
		Name:         "actionLoggerHook",
		Desc:         "Use to collect UI action logs",
		Contacts:     []string{"chromeos-software-engprod@google.com", "alvinjia@google.com", "mattlui@google.com"},
		BugComponent: "b:918425", // ChromeOS > Engprod > Software
		Impl:         &actionLoggerhook{},
	})
}

type actionLoggerhook struct {
	shouldRun     bool
	cl            *rpc.Client
	serviceClient pb.ActionLoggerServiceClient
}

// SetUp will be called during root fixture Setup.
func (h *actionLoggerhook) SetUp(ctx context.Context, s *HookState) error {
	h.shouldRun = false
	if alcommon.ShouldRun.Value() != "true" {
		return nil
	}

	var err error
	h.cl, err = rpc.Dial(s.FixtContext(), s.DUT(), s.RPCHint())
	if err != nil {
		s.fixtState.Log("Cannot start rpc service for action logger: ", err)
		return nil
	}
	h.serviceClient = pb.NewActionLoggerServiceClient(h.cl.Conn)
	h.shouldRun = true
	return nil
}

// Reset will be called during root fixture Reset.
func (h *actionLoggerhook) Reset(ctx context.Context) error {
	return nil
}

// PreTest will be called during root fixture PreTest.
func (h *actionLoggerhook) PreTest(ctx context.Context, s *HookTestState) error {
	if !h.shouldRun {
		return nil
	}
	var req empty.Empty
	if _, err := h.serviceClient.Reset(ctx, &req); err != nil {
		s.fixtTestState.Log("Failed to reset action logger: ", err)
		return nil
	}
	return nil
}

// PostTest will be called during root fixture PostTest.
func (h *actionLoggerhook) PostTest(ctx context.Context, s *HookTestState) error {
	if !h.shouldRun {
		return nil
	}

	var req empty.Empty
	if _, err := h.serviceClient.Save(ctx, &req); err != nil {
		s.fixtTestState.Log("Failed to save action logs: ", err)
		return nil
	}
	return nil
}

// TearDown will be called during root fixture TearDown.
func (h *actionLoggerhook) TearDown(ctx context.Context, s *HookState) error {
	if h.cl == nil {
		return nil
	}
	h.cl.Close(s.FixtContext())
	return nil
}
