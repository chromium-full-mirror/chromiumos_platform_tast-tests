// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package proxy

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
)

// Proxy defines all interfaces to abstract a proxy running in the test environment.
type Proxy interface {
	// Close closes proxy.
	Close(ctx context.Context) error

	// Connect connects proxy to ash-chrome.
	Connect(ctx context.Context, cr *chrome.Chrome) error

	// IsRunning returns whether the proxy is running.
	IsRunning() bool

	// RootCertificate returns the file path and the type of the root certificate then ensures its existence.
	RootCertificate(ctx context.Context) (string, string, error)

	// ProxyAddress returns the proxy address to be set in browser.
	ProxyAddress() string

	// DumpHTTPFlow returns the HTTP flow on success, or an error if anything goes wrong.
	DumpHTTPFlow(ctx context.Context, reset, saveToFile bool) (*DumpHTTPResponse, error)
}

// DumpHTTPResponse is used to return DumpHTTPFlow response.
type DumpHTTPResponse struct {
	URLs []string `json:"url"`
}
