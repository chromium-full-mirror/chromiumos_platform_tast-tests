// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testhooks

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/arc"
	arcutil "go.chromium.org/tast-tests/cros/local/network/arc"
	"go.chromium.org/tast/core/testing"
)

// dumpARCOnFailureHook implements hook interface.
type dumpARCOnFailureHook struct {
	noopTearDownMixin

	a            *arc.ARC
	errorHandler func(string)
}

func (h *dumpARCOnFailureHook) name() string {
	return "dump_arc_on_failure"
}

func (h *dumpARCOnFailureHook) setUp(ctx context.Context) error {
	if h.a == nil {
		testing.ContextLogf(ctx, "Skip setup of %s due to empty arc.ARC", h.name())
	}
	h.errorHandler = arcutil.CreateNetworkDumpsysErrorHandler(ctx, h.a)
	return nil
}

func (h *dumpARCOnFailureHook) onError(errMsg string) {
	if h.errorHandler == nil {
		return
	}
	h.errorHandler(errMsg)
}

func (h *dumpARCOnFailureHook) OnFatal(errMsg string) {
	if h.errorHandler == nil {
		return
	}
	h.errorHandler(errMsg)
}
