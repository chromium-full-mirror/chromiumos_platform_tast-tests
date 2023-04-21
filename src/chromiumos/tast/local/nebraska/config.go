// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package nebraska

import "context"

// Option is a self-referential function can be used to configure Nebraska.
// See https://commandcenter.blogspot.com.au/2014/01/self-referential-functions-and-design.html
// for details about this pattern.
type Option func(m *MutableConfig) error

// Config contains configurations for Nebraska.
// Once retrieved by NewConfig() it should be used to read from not to modify.
type Config struct {
	m MutableConfig
}

// MutableConfig holds pieces of configuration that are set with Options.
type MutableConfig struct {
	Args []string
	// RuntimeRoot is Nebraska's runtime root.
	RuntimeRoot string
	// LogFile is Nebraska's log file location.
	LogFile string
	// ConfigureUpdateEngine is called once the port of Nebraska is known.
	// If configured to do so by the corresponding Options,
	// it will point update-engine towards the nebraska instance.
	ConfigureUpdateEngine func(port int) error
	// Cleanups to be run after stopping Nebraska itself.
	Cleanup []func(ctx context.Context) error
}

// NewConfig creates new configuration.
func NewConfig(options []Option) (*Config, error) {
	config := &Config{
		m: MutableConfig{
			Args:                  []string{},
			RuntimeRoot:           "/run/nebraska",
			LogFile:               "/tmp/nebraska.log",
			Cleanup:               []func(ctx context.Context) error{},
			ConfigureUpdateEngine: func(port int) error { return nil },
		},
	}

	// The temp dir option is hard coded because tests should not run Nebraska in its default directory.
	// It is safer for every test to create their own temporary directory with Nebraska.
	options = append(options, runInTmpDir())

	for _, option := range options {
		if err := option(&config.m); err != nil {
			return nil, err
		}
	}
	return config, nil
}
