// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ex350

import (
	"context"
	"fmt"

	"go.chromium.org/tast-tests/cros/remote/compliance"
	"go.chromium.org/tast/core/errors"
)

// Controller wraps a WindowsHost to provide common functionality for controlling
// an EX350 compliance tester.
type Controller struct {
	Host *compliance.WindowsHost
}

// NewController initializes a new EX350 controller.
func NewController(host *compliance.WindowsHost) *Controller {
	return &Controller{
		Host: host,
	}
}

// RunCompliance runs the compliance EX350 CLI with the supplied arguments
func (w *Controller) RunCompliance(ctx context.Context, generator, vif, testName, reportDirectory string) (string, error) {
	compliance := w.Host.Host.CommandContext(ctx, `C:\Users\CrOSECMinion\Documents\compliance.ps1`, generator, vif, fmt.Sprintf(`'%s'`, testName), reportDirectory)
	out, err := compliance.CombinedOutput()
	if err != nil {
		return "", errors.Wrap(err, string(out))
	}
	return string(out), err
}
