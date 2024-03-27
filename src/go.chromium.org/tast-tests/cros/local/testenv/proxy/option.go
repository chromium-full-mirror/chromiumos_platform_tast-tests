// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package proxy

// Option is a function that can be used to config MitmProxy.
type Option func(*MitmProxy) error

// ScriptPath is an option to set scriptPaths in MitmProxy.
func ScriptPath(paths ...string) Option {
	return func(mp *MitmProxy) error {
		for _, p := range paths {
			mp.scriptPaths = append(mp.scriptPaths, p)
		}
		return nil
	}
}

// CustomOptions is an option to set options in MitmProxy.
func CustomOptions(opts ...string) Option {
	return func(mp *MitmProxy) error {
		for _, opt := range opts {
			mp.options = append(mp.options, opt)
		}
		return nil
	}
}

// OutDir is an option to set outDir where all the logs are saved.
// Practically this option should not need to be set in most tests except for a RPC service that starts a proxy using its own background context.
func OutDir(path string) Option {
	return func(mp *MitmProxy) error {
		mp.outDir = path
		return nil
	}
}

// HealthCheck is an option to check the proxy server health when it is started.
// The magic domain will be used for checking under the hood.
// If an allowlist or blocklist is set to block this domain, users can set `allow=false` to bypass the check.
// default: true
func HealthCheck(allow bool) Option {
	return func(mp *MitmProxy) error {
		mp.healthCheck = allow
		return nil
	}
}
