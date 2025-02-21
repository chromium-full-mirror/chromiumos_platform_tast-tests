// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package teledyne

import (
	"context"

	"go.chromium.org/tast-tests/cros/remote/compliance"
	"go.chromium.org/tast/core/errors"
)

// Controller wraps a WindowsHost to provide common functionality for controlling
// a teledyne compliance tester.
type Controller struct {
	Host *compliance.WindowsHost
}

// NewController initializes a new teledyne controller.
func NewController(host *compliance.WindowsHost) *Controller {
	return &Controller{
		Host: host,
	}
}

// RunCompliance runs the compliance teledyne CLI with the supplied arguments
func (w *Controller) RunCompliance(ctx context.Context) (string, error) {
	compliance := w.Host.Host.CommandContext(ctx, `C:\Users\Public\Compliance\Teledyne\USBCompliance.ps1`)
	out, err := compliance.CombinedOutput()
	if err != nil {
		return "", errors.Wrap(err, string(out))
	}
	return string(out), err
}
