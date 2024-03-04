// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ex350

import (
	"go.chromium.org/tast-tests/cros/remote/compliance"
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
