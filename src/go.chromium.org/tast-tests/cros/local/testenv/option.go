// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testenv

// Fixed keys of the env vars used to set up BaseEnv.vars
const (
	hostMapVar     = "HostMapVar" // Value should be `testing.RegisterVarString(<yaml_data_file>).Value()`. eg, "testenv.FakeEnv.HostMap"
	redirectMapVar = "RedirectMapVar"
)

// Option is a function that can be used to configure BaseEnv.
type Option func(*BaseEnv) error

// HostMap is an option to set the map of host aliases to names from the passed-in yaml data file.
func HostMap(val string) Option {
	return func(b *BaseEnv) error {
		b.vars[hostMapVar] = val
		return nil
	}
}

// RedirectMap is an option to set up the host redirection rules.
// The arg only accepts host alias, not directly uses host names to hide the host details.
// Example, {"example-prod": "example-preprod"}
func RedirectMap(redirects map[string]string) Option {
	return func(b *BaseEnv) error {
		// Only the last one will be used if this option is passed in more than once.
		b.vars[redirectMapVar] = redirects
		return nil
	}
}
