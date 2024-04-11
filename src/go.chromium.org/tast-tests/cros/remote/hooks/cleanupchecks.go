// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package hooks contains code for support adding custom hooks to root fixture.
package hooks

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/cleanupchecks"
	cs "go.chromium.org/tast-tests/cros/services/cros/cleanupchecks"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
)

func init() {
	addHook(&Hook{
		Name:         "cleanupChecksHook",
		Desc:         "Executes clean up checks after each test",
		Contacts:     []string{"tast-owner@google.com", "alexanderhartl@google.com"},
		BugComponent: "b:1170223", // ChromeOS > Software > Commercial (Enterprise) > EngProd
		Impl:         &cleanUpCheckHook{},
	})
}

type cleanUpCheckHook struct {
	cleanUpChecksLevel cleanupchecks.LevelEnum
	dataMap            map[string]*cs.PreTestData
}

// SetUp will be called during root fixture Setup.
// The cleanupchecks.CleanUpChecksLevel runtime variable is used to determine if this hook is enabled
// and which clean up checks will be executed.
func (h *cleanUpCheckHook) SetUp(ctx context.Context, s *HookState) error {
	switch cleanUpChecksLevel := cleanupchecks.CleanUpChecksLevel.Value(); cleanUpChecksLevel {
	case "1":
		h.cleanUpChecksLevel = cleanupchecks.Fast
	case "2":
		h.cleanUpChecksLevel = cleanupchecks.Medium
	case "3":
		h.cleanUpChecksLevel = cleanupchecks.Slow
	default:
		h.cleanUpChecksLevel = cleanupchecks.Disabled
	}

	return nil
}

// Reset will not do anything for this hook.
func (h *cleanUpCheckHook) Reset(ctx context.Context) error {
	return nil
}

// PreTest will record the current DUT status.
func (h *cleanUpCheckHook) PreTest(ctx context.Context, s *HookTestState) error {
	if h.cleanUpChecksLevel == cleanupchecks.Disabled {
		return nil
	}

	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		return errors.Wrap(err, "failed to connect to the RPC service on the DUT")
	}
	defer cl.Close(ctx)

	cleanUpCheckService := cs.NewCleanUpChecksServiceClient(cl.Conn)

	response, err := cleanUpCheckService.RecordStateBeforeTestFromDUT(ctx, &cs.RecordStateBeforeTestFromDUTRequest{})
	if err != nil {
		return errors.Wrap(err, "failed to collect the data from DUT before the test run")
	}
	h.dataMap = response.PreTestDataMap

	return nil
}

// PostTest will check current DUT status and perform certain cleanup.
func (h *cleanUpCheckHook) PostTest(ctx context.Context, s *HookTestState) error {
	if h.cleanUpChecksLevel == cleanupchecks.Disabled {
		return nil
	}

	cl, err := rpc.Dial(s.TestContext(), s.DUT(), s.RPCHint())
	if err != nil {
		return errors.Wrap(err, "failed to connect to the RPC service on the DUT")
	}
	defer cl.Close(s.TestContext())

	cleanUpCheckService := cs.NewCleanUpChecksServiceClient(cl.Conn)
	_, err = cleanUpCheckService.ExecuteCleanUpChecksOnDUT(ctx, &cs.ExecuteCleanUpChecksOnDUTRequest{PreTestDataMap: h.dataMap})
	if err != nil {
		return errors.Wrap(err, "failed to execute clean up checks on DUT")
	}

	return nil
}

// TearDown will not do anything.
func (h *cleanUpCheckHook) TearDown(ctx context.Context, s *HookState) error {
	return nil
}
