// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package utils defines utility functions common to local and remote tests.
package utils

import (
	"context"
	"net"
	"os"
	"regexp"
	"strings"
	"unicode"

	"go.chromium.org/tast/core/caller"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Suffix names for forward compatibility.
const (
	// CompanionSuffixPcap is a companion suffix for the pcap.
	CompanionSuffixPcap = "-pcap"
	// CompanionSuffixRouter is a companion suffix for the router.
	CompanionSuffixRouter = "-router"
	// CompanionSuffixTablet is a companion suffix for the tablet.
	CompanionSuffixTablet = "-tablet"
)

// CollectFirstErr collects the first error into firstErr and logs the others.
// This can be useful when you have several steps in a function but cannot early
// return on error. e.g. cleanup functions.
func CollectFirstErr(ctx context.Context, firstErr *error, err error) {
	if err == nil {
		return
	}
	testing.ContextLogf(ctx, "Error in %s: %s", caller.Get(2), err)
	if *firstErr == nil {
		*firstErr = err
	}
}

// ErrCompanionHostname is the error of deriving default companion device hostname from dut's hostname.
// e.g. when DUT is connected with IP address.
var ErrCompanionHostname = errors.New("cannot derive default companion device hostname")

// CompanionDeviceHostname derives the hostname of companion device from test target hostname
// with the convention in Autotest.
// (see server/cros/dnsname_mangler.py in Autotest)
func CompanionDeviceHostname(dutHost, suffix string) (string, error) {
	// Try split out port part.
	if host, _, err := net.SplitHostPort(dutHost); err == nil {
		dutHost = host
	}

	if ip := net.ParseIP(dutHost); ip != nil {
		// Cannot derive companion hostname from IP. Return error.
		return "", ErrCompanionHostname
	}

	// Companion device hostname convention: append suffix after the first sub-domain string.
	hostname := strings.SplitN(dutHost, ".", 2)
	// In the multidut case, hostname is in the form of <nameNN>a or <nameNN>b but the companion hostname is <nameNN><suffix>.
	// One exception is devices with "-tablet" suffix. They still keep 'a' and 'b' before the suffix.
	if regexp.MustCompile("\\w\\d+\\w+$").Match([]byte(hostname[0])) && suffix != CompanionSuffixTablet {
		hostname[0] = strings.TrimRightFunc(hostname[0], unicode.IsLetter)
	}
	hostname[0] = hostname[0] + suffix
	return strings.Join(hostname, "."), nil
}

// IsCloudBot returns true if it running on cloudbots vm.
func IsCloudBot() bool {
	return strings.HasPrefix(os.Getenv("SWARMING_BOT_ID"), "cloudbots-")
}
