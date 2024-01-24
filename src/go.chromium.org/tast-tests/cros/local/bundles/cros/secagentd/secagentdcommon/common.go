// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package secagentdcommon contains shared general helpers used in secagentd tast tests.
package secagentdcommon

import (
	"context"
	"os"
	"path/filepath"

	xdr "go.chromium.org/chromiumos/xdr/secagentd"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
)

const kernelTraceFile = "/sys/kernel/debug/tracing/trace"

// CheckCommon verifies that the common message fields are filled with appropriate values.
func CheckCommon(common *xdr.CommonEventVariantDataFields) error {
	if common.GetCreateTimestampUs() == 0 {
		return errors.New("CreateTimestampUs field not set")
	}
	deviceUser := common.GetDeviceUser()
	if common.DeviceUser == nil || deviceUser == "Unknown" {
		return errors.Errorf("invalid username: %s", deviceUser)
	}
	return nil
}

// ClearKernelTrace clears out the kernel tracing buffer.
func ClearKernelTrace(ctx context.Context) error {
	if err := os.Truncate(kernelTraceFile, 0); err != nil {
		return errors.Wrapf(err, "failed to clear %q", kernelTraceFile)
	}
	return nil
}

// OnErrorSaveKernelTrace saves off a copy of the kernel trace on test failure.
// USAGE: defer OnErrorSaveKernelTrace(ctx, s)
func OnErrorSaveKernelTrace(ctx context.Context, outDir string, hasError func() bool) error {
	if !hasError() {
		return nil
	}
	outFile := filepath.Join(outDir, "kernel_trace")
	if err := fsutil.CopyFile(kernelTraceFile, outFile); err != nil {
		return errors.Wrapf(err, "failed to copy %q to %q:", kernelTraceFile, outFile)

	}
	return nil
}
