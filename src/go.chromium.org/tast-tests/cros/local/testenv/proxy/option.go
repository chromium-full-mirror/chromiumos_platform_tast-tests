// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package proxy

import (
	"fmt"
	"strings"

	"go.chromium.org/tast/core/errors"
	"gopkg.in/yaml.v2"
)

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
			if opt != "" {
				mp.options = append(mp.options, opt)
			}
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

// DumpHTTPFlow is an option to switch dumpHttpFlow when proxy starts.
// Default value of allow is false.
func DumpHTTPFlow(enable bool) Option {
	return func(mp *MitmProxy) error {
		mp.dumpHTTPFlowEnabled = enable
		return nil
	}
}

// URLRedirect is an option to set up the `map_remote` proxy option that redirects remote destination to another remote URL.
//
// The simple format is a map of { urlregexp: replacement }, where
//
//	`urlregexp` is a regexp that defines what gets placed in the request URLs. It matches the full URL string including schema, host, port, path and all URL components.
//	`replacement` is a string liternal that gets replaced in.
//
// Examples:
//
//	{ `.*\.org$`: `https://foo.bar/about` } - Map all requests ending with .org to https://foo.bar/about
//	{ `//foo.org/`: `//bar.org/` } - Map all requests from foo.org to bar.org
func URLRedirect(urlMap map[string]string) Option {
	var lines []string
	if len(urlMap) > 0 {
		lines = append(lines, "map_remote: ")
		for pattern, replacement := range urlMap {
			lines = append(lines, fmt.Sprintf(` - '|%s|%s'`, pattern, replacement))
		}
	}
	return CustomOptions(strings.Join(lines, "\n"))
}

// DumpFull is an option to write out a full dump of mitmproxy to a file.
// It defaults to false as it may use up a lot of space for every one unless really necessary.
func DumpFull(enable bool) Option {
	return func(mp *MitmProxy) error {
		mp.dumpFull = enable
		return nil
	}
}

// Allowlist configures HTTP flow filtering rules for MitmProxy.
// It accepts a list of hostnames.
//
// Filter follows these rules:
// 1. Only traffic to the specified hosts is allowed.
// 2. Otherwise, the traffic is blocked and return 403.
//
// Sample for hosts:
//
//	"www.example.com", "api.google.com"
func Allowlist(hosts []string) Option {
	return func(mp *MitmProxy) error {
		mp.allowedHosts = append(mp.allowedHosts, hosts...)

		config := map[string][]string{
			"allowedHosts": hosts,
		}

		yamlConfig, err := yaml.Marshal(&config)
		if err != nil {
			return errors.Wrapf(err, "failed to marshal allowedHosts: %s", hosts)
		}
		mp.options = append(mp.options, string(yamlConfig))
		return nil
	}
}

// Ignorelist configures MitmProxy ignore_hosts.
// It accepts a list of hostnames.
// MitmProxy ignores any host in the list without processing it.
// It is useful if you want to exempt some traffic that is protected using certificate pinning
// or traffic that you don't really care about for what it is testing.
func Ignorelist(hosts []string) Option {
	return func(mp *MitmProxy) error {
		if len(hosts) == 0 {
			return errors.New("failed to setup IgnoreHosts since host is empty")
		}

		if len(mp.ignoredHosts) != 0 {
			return errors.New("failed to setup IgnoreHosts since it has been setup")
		}

		mp.ignoredHosts = hosts
		config := map[string][]string{
			"ignore_hosts": hosts,
		}
		yamlConfig, err := yaml.Marshal(&config)
		if err != nil {
			return errors.Wrapf(err, "failed to marshal IgnoreHosts: %s", hosts)
		}

		mp.options = append(mp.options, string(yamlConfig))

		return nil
	}
}

// CustomCA is an option to use custom CA or not.
// If enable, then mitmproxy will use a CA defined in private vars folder.
// Otherwise, CA is generated during runtime.
// The default value is false.
func CustomCA(enable bool) Option {
	return func(mp *MitmProxy) error {
		mp.customCA = enable
		return nil
	}
}
