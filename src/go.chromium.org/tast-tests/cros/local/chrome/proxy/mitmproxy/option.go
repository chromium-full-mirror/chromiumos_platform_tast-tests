// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mitmproxy

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

// OutDir is an option to set outDir in MitmProxy.
func OutDir(path string) Option {
	return func(mp *MitmProxy) error {
		mp.outDir = path
		return nil
	}
}
