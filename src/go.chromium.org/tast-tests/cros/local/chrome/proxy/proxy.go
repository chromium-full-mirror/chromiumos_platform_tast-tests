// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package proxy defines proxy interface on Chrome for testing.
package proxy

import "context"

// Proxy defines interface of proxy.
type Proxy interface {
	// Start starts the proxy.
	Start(ctx context.Context) error

	// Close closes proxy.
	Close(ctx context.Context) error

	// IsRunning returns whether the proxy is running.
	IsRunning() bool

	// RootCertificate returns the file path of the root certificate and ensures its existence.
	RootCertificate(ctx context.Context) (string, error)

	// ProxyAddress returns the proxy address to be set in browser.
	ProxyAddress() string
}
