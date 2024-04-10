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
// When allow is true, user should pass dumpHTTPFlowAddonPath as well.
// TODO(b/319732303): remove dumpHTTPFlowAddonPath later.
func DumpHTTPFlow(allow bool, dumpHTTPFlowAddonPath string) Option {
	return func(mp *MitmProxy) error {
		if allow && len(dumpHTTPFlowAddonPath) == 0 {
			return errors.New("Please pass dumpHTTPFlowAddonPath to enable dumpHTTPFlow")
		}

		if !allow && len(dumpHTTPFlowAddonPath) != 0 {
			return errors.New("You cannot pass dumpHTTPFlowAddonPath when disable dumpHTTPFlow")
		}

		mp.dumpHTTPFlowEnabled = allow
		mp.dumpHTTPFlowAddonPath = dumpHTTPFlowAddonPath
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
// 2. Otherwise, the traffic is blocked and return 500.
//
// Sample for hosts:
//
//	"www.example.com", "api.google.com"
//
// TODO(b/319732303): remove trafficFilterAddonPath later.
func Allowlist(hosts []string, trafficFilterAddonPath string) Option {
	return func(mp *MitmProxy) error {
		if len(hosts) == 0 {
			return errors.New("failed to filter HTTP flow since allowedHosts is empty")
		}

		if len(trafficFilterAddonPath) == 0 {
			return errors.New("failed to filter HTTP flow since trafficFilterAddonPath is empty")
		}

		mp.allowedHosts = append(mp.allowedHosts, hosts...)
		mp.scriptPaths = append(mp.scriptPaths, trafficFilterAddonPath)

		config := map[string][]string{
			"allowedHosts": hosts,
		}

		yamlConfig, err := yaml.Marshal(&config)
		if err != nil {
			return errors.Wrapf(err, "fail to marshal allowedHosts: %s", hosts)
		}
		mp.options = append(mp.options, string(yamlConfig))
		return nil
	}
}
