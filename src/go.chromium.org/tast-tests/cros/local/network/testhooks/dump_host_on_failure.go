// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testhooks

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/network/dumputil"
)

// dumpHostOnFailureHook implements network/hooks.hook interface.
type dumpHostOnFailureHook struct {
	noopTearDownMixin

	errorHandler func(string)
}

func (h *dumpHostOnFailureHook) name() string {
	return "dump_host_on_failure"
}

// setUp creates the error handler.
func (h *dumpHostOnFailureHook) setUp(ctx context.Context) error {
	h.errorHandler = dumputil.CreateErrorHandler(ctx)
	return nil
}

func (h *dumpHostOnFailureHook) onError(errMsg string) {
	h.errorHandler(errMsg)
}

func (h *dumpHostOnFailureHook) OnFatal(errMsg string) {
	h.errorHandler(errMsg)
}
