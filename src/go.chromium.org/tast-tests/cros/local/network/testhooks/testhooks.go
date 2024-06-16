// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testhooks

import (
	"context"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// RunNetworkTestHooks runs the setup of hooks in order, and returns a hooksEnv
// struct on success. Typically, the caller should defer hooksEnv.TearDown (or
// hooksEnv.TearDownWithLogFailures), and register hooksEnv.OnErrorHandler and
// hooksEnv.OnFatalHandler with s.AttachErrorHandlers(). Example:
//
//	hookEnv, err := testhooks.RunNetworkTestHooks(ctx,
//	  testhooks.NewSaveNetLogHook(),
//	  testhooks.NewDumpHostOnFailuresHook(),
//	  testhooks.NewTcpdumpHook(),
//	)
//	if err != nil {
//	  s.Fatal("Failed to run network test hooks: ", err)
//	}
//	s.AttachErrorHandlers(hookEnv.OnErrorHandler, hookEnv.OnFatalHandler)
//	defer hookEnv.TearDownWithLogFailures(cleanupCtx, s.HasError)
//
// If this function is called in a fixture, s.AttachErrorHandlers should still
// be called in a test instead of the PreTest() of the fixture, otherwise the
// s.Error()/s.Fatal() in the test won't trigger these handlers.
func RunNetworkTestHooks(ctx context.Context, hooks ...hook) (*hookEnv, error) {
	hookData := &hookEnv{
		hasTornDown: false,
	}

	success := false
	defer func() {
		if success {
			return
		}
		if err := hookData.TearDown(ctx, nil); err != nil {
			testing.ContextLog(ctx, "Failed to clean up hooks after run failures: ", err)
		}
	}()

	for _, h := range hooks {
		if err := h.setUp(ctx); err != nil {
			return nil, errors.Wrapf(err, "failed to run hook %s", h.name())
		}
		hookData.hooks = append([]hook{h}, hookData.hooks...)
	}

	success = true
	return hookData, nil
}

type hookEnv struct {
	hooks       []hook
	hasTornDown bool
}

type hasErrorFunc func() bool

// TearDown runs tearDown of the hooks in the reverse order. hasError will be
// passed to each hook, typically it should be s.HasError.
func (h *hookEnv) TearDown(ctx context.Context, hasError hasErrorFunc) error {
	h.hasTornDown = true
	var errs []error
	for _, hook := range h.hooks {
		if err := hook.tearDown(ctx, hasError); err != nil {
			errs = append(errs, errors.Wrapf(err, "failed to tear down hook %s", hook.name()))
		}
	}
	return errors.Join(errs...)
}

// TearDownWithLogFailures runs tearDown of the hooks in the reverse order.
// hasError will be passed to each hook, typically it should be s.HasError.
// Different with TearDown, on any failure of the hook, this function will leave
// a log instead of returning the err. This is helpful if the caller does not
// need to handle the returned err.
func (h *hookEnv) TearDownWithLogFailures(ctx context.Context, hasError hasErrorFunc) {
	if err := h.TearDown(ctx, hasError); err != nil {
		testing.ContextLog(ctx, "Failed to run tear down network test hooks: ", err)
	}
	return
}

// OnErrorHandler runs the onError handlers of the hooks in the reverse order.
// Use this function in s.AttachErrorHandlers().
func (h *hookEnv) OnErrorHandler(errMsg string) {
	if h.hasTornDown {
		return
	}
	for _, hook := range h.hooks {
		hook.onError(errMsg)
	}
}

// OnFatalHandler runs the OnFatal handlers of the hooks in the reverse order.
// Use this function in s.AttachErrorHandlers().
func (h *hookEnv) OnFatalHandler(errMsg string) {
	if h.hasTornDown {
		return
	}
	for _, hook := range h.hooks {
		hook.OnFatal(errMsg)
	}
}
