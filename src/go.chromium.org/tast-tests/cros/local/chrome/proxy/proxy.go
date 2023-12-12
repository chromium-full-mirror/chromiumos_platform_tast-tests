// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package proxy defines proxy interface on Chrome for testing.
package proxy

import (
	"context"

	"go.chromium.org/tast/core/testing"
)

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

// EnabledVar indicates whether to enable the proxy.
var EnabledVar = testing.RegisterVarString(
	"proxy.enable",
	"false",
	"proxy.enable indicates whether to enable the proxy",
)

// BinaryPathVar indicates the path of proxy binary.
var BinaryPathVar = testing.RegisterVarString(
	"proxy.binaryPath",
	"",
	"proxy.binaryPath indicates the path of proxy binary",
)

// PortVar indicates the port of proxy.
var PortVar = testing.RegisterVarString(
	"proxy.port",
	"",
	"proxy.port indicates the port of proxy",
)

// OutDirVar indicates the outcome directory of proxy testing.
var OutDirVar = testing.RegisterVarString(
	"proxy.outDir",
	"",
	"proxy.outDir indicates the outcome directory of proxy testing",
)

// ConfDirVar indicates the configuration directory of proxy testing.
var ConfDirVar = testing.RegisterVarString(
	"proxy.confDir",
	"",
	"proxy.confDir indicates the configuration directory of proxy testing",
)

// CompressDumpVar indicates whether to compress traffic dump file.
var CompressDumpVar = testing.RegisterVarString(
	"proxy.compressDump",
	"true",
	"proxy.compressDump indicates whether to compress traffic dump file",
)

// RemoveCertVar indicates whether to remove cert after testing is completed.
var RemoveCertVar = testing.RegisterVarString(
	"proxy.removeCert",
	"true",
	"proxy.removeCert indicates whether to remove cert after testing is completed",
)

// ScriptPathVar indicates the path of addon script.
var ScriptPathVar = testing.RegisterVarString(
	"proxy.scriptPath",
	"",
	"proxy.scriptPath indicates the path of addon script",
)
