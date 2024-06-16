// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testhooks

import (
	"context"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/local/logsaver"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// saveNetLogHook implements hook interface.
type saveNetLogHook struct {
	noErrorHandlersMixin

	logMarker *logsaver.Marker
}

func (h *saveNetLogHook) name() string {
	return "save_net_log"
}

func (h *saveNetLogHook) setUp(ctx context.Context) error {
	logMarker, err := logsaver.NewMarker("/var/log/net.log")
	if err != nil {
		return errors.Wrap(err, "failed to create log saver for net.log")
	}
	h.logMarker = logMarker
	return nil
}

func (h *saveNetLogHook) tearDown(ctx context.Context, hasError func() bool) error {
	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return errors.New("failed to get ContextOutDir")
	}
	if err := h.logMarker.Save(filepath.Join(outDir, "net.log")); err != nil {
		return errors.Wrap(err, "failed to store log to net.log")
	}
	return nil
}
