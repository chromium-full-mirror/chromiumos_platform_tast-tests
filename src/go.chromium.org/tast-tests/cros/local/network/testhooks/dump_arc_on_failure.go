// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testhooks

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/arc"
	arcutil "go.chromium.org/tast-tests/cros/local/network/arc"
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
	h.errorHandler = arcutil.CreateNetworkDumpsysErrorHandler(ctx, h.a)
	return nil
}

func (h *dumpARCOnFailureHook) onError(errMsg string) {
	h.errorHandler(errMsg)
}

func (h *dumpARCOnFailureHook) OnFatal(errMsg string) {
	h.errorHandler(errMsg)
}
