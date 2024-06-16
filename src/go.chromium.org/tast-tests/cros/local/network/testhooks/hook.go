// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testhooks

import "context"

type hook interface {
	// name returns the name of this hook.
	name() string

	// setUp does the setUp process of this hook. Typically this will be called at
	// the beginning of a test.
	setUp(ctx context.Context) error

	// tearDown does the teardown process of this hook. hasError can be nil.
	// Typically this will be called at the end of a test.
	tearDown(ctx context.Context, hasError func() bool) error

	// OnError is the error handler of this hook. Typically this will be called
	// every time s.Error() is called.
	onError(errMsg string)

	// OnFatal is the error handler of this hook. Typically this will be called
	// every time s.Fatal() is called.
	OnFatal(errMsg string)
}

// noopSetUpMixin is a mixin struct for a struct implements the hook interface.
// It does nothing in setUp.
type noopSetUpMixin struct {
}

func (n *noopSetUpMixin) setUp(ctx context.Context) error { return nil }

// noopTearDownMixin is a mixin struct for a struct implements the hook
// interface. It does nothing in TearDown.
type noopTearDownMixin struct {
}

func (n *noopTearDownMixin) tearDown(ctx context.Context, hasError func() bool) error { return nil }

// noErrorHandlersMixin is a mixin struct for a struct implements the hook
// interface. It does nothing in error handlers.
type noErrorHandlersMixin struct {
}

func (n *noErrorHandlersMixin) onError(errMsg string) {}
func (n *noErrorHandlersMixin) OnFatal(errMsg string) {}
