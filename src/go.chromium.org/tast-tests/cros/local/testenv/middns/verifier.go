// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package middns implements the middle layer DNS server using dnsmasq to redirect client requests for e2e testing.
package middns

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// QueryMatcher is a matcher that checks certain query info in `dig` output.
type QueryMatcher func(out string) bool

// ByHostAndIP returns whether the given hostname and IP are found in the dig answer section.
func ByHostAndIP(host, ip string) QueryMatcher {
	return func(out string) bool {
		re := regexp.MustCompile(
			fmt.Sprintf(`%s[^\n]+\b%s\b`, regexp.QuoteMeta(host), regexp.QuoteMeta(ip)))
		return re.MatchString(out)
	}
}

// Not negates a given matcher.
func Not(m QueryMatcher) QueryMatcher {
	return func(out string) bool {
		return !m(out)
	}
}

// digMatch runs `dig` and check if the output satisfies the given matcher.
func digMatch(ctx context.Context, host string, m QueryMatcher) error {
	out, err := testexec.CommandContext(ctx, "dig", host).Output()
	if err != nil {
		return errors.Wrap(err, "failed to run dig")
	}
	if !m(string(out)) {
		return errors.Errorf("failed to dig match host: %v", host)
	}
	return nil
}

// VerifyQuery checks if a DNS query for the given host satisfies the given matcher in `dig` output until the given timeout.
func VerifyQuery(ctx context.Context, host string, m QueryMatcher, timeout time.Duration) error {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return digMatch(ctx, host, m)
	}, &testing.PollOptions{Timeout: timeout, Interval: 2 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to verify host redirect")
	}
	return nil
}
