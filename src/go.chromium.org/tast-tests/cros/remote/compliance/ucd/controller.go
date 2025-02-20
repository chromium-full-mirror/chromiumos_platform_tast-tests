// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ucd

import (
	"context"

	"go.chromium.org/tast-tests/cros/remote/compliance"
	"go.chromium.org/tast/core/errors"
)

// Controller wraps a WindowsHost to provide common functionality for controlling
// a ucd compliance tester.
type Controller struct {
	Host *compliance.WindowsHost
}

// NewController initializes a new ucd controller.
func NewController(host *compliance.WindowsHost) *Controller {
	return &Controller{
		Host: host,
	}
}

// RunCompliance runs the compliance UCD CLI with the supplied arguments
func (w *Controller) RunCompliance(ctx context.Context) (string, error) {
	compliance := w.Host.Host.CommandContext(ctx, `python`, `C:\Users\Public\Compliance\UCD\Unigraf\sdk\python\UniTAP\examples\cli_DP14.py`)
	out, err := compliance.CombinedOutput()
	if err != nil {
		return "", errors.Wrap(err, string(out))
	}
	return string(out), err
}
