// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package proxy

import (
	"context"
)

// Proxy defines all interfaces to abstract a proxy running in the test environment.
type Proxy interface {
	// Close closes proxy.
	Close(ctx context.Context) error

	// IsRunning returns whether the proxy is running.
	IsRunning() bool

	// RootCertificate returns the file path of the root certificate and ensures its existence.
	RootCertificate(ctx context.Context) (string, error)

	// ProxyAddress returns the proxy address to be set in browser.
	ProxyAddress() string

	// DumpHTTPFlow returns the HTTP flow on success, or an error if anything goes wrong.
	DumpHTTPFlow(ctx context.Context, reset, saveToFile bool) (*DumpHTTPResponse, error)
}

// DumpHTTPResponse is used to return DumpHTTPFlow response.
type DumpHTTPResponse struct {
	URLs []string `json:"url"`
}
