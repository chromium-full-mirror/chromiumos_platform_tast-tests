// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package browser implements a layer of abstraction over Ash and Lacros Chrome
// instances.
package browser

import (
	"go.chromium.org/tast-tests/cros/local/chrome/internal/cdputil"
	"go.chromium.org/tast-tests/cros/local/chrome/internal/driver"
)

// Browser consists primarily of a Chrome session.
type Browser struct {
	sess                     *driver.Session
	autotestPrivateSupported bool
}

// New creates a new Browser instance from an existing Chrome session.
func New(sess *driver.Session, autotestPrivateSupported bool) *Browser {
	return &Browser{sess, autotestPrivateSupported}
}

// CreateTargetOption is cpdutil.CreateTargetOption.
type CreateTargetOption = cdputil.CreateTargetOption

// WithNewWindow behaves like cpdutil.WithNewWindow.
func WithNewWindow() CreateTargetOption {
	return cdputil.WithNewWindow()
}

// TraceOption is cpdutil.TraceOption.
type TraceOption = cdputil.TraceOption

// DisableSystrace behaves like cpdutil.DisableSystrace.
func DisableSystrace() TraceOption {
	return cdputil.DisableSystrace()
}
