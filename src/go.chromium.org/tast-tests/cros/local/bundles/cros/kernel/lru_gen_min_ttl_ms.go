// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package kernel

import (
	"context"
	"os"
	"strings"
	"time"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/sysutil"
	"go.chromium.org/tast-tests/cros/local/upstart"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LruGenMinTTLMs,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Ensures the system has the expected value for /sys/kernel/mm/lru_gen/min_ttl_ms",
		Contacts: []string{
			"chromeos-memory@google.com",
			"dianders@google.com",
		},
		BugComponent: "b:167286",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
	})
}

const (
	// expectedLruGenMinTTLMs is the value we expect
	expectedLruGenMinTTLMs = "250"

	// enabledFeature is the name of the test feature in platform-features.json.
	// It should always match the name in that file exactly.
	enabledFeature = "CrOSLateBootLruMinTtlMs250"
)

// LruGenMinTTLMs checks to make sure /sys/kernel/mm/lru_gen/min_ttl_ms got
// set correctly. At the moment this is set by featured, so we need to make
// sure Chrome has started for the test to pass.
func LruGenMinTTLMs(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	ver, _, err := sysutil.KernelVersionAndArch()
	if err != nil {
		s.Fatal("Failed to get kernel version and arch: ", err)
	}

	// Although we've backported MGLRU to all of our kernels, we haven't
	// backported the `min_ttl_ms` past 5.10. We'll just consider this a
	// test pass if it's an old kernel.
	if !ver.IsOrLater(5, 10) {
		return
	}

	// Restart featured. featured only sets the flag on the first login
	// after it starts, since we assume clients of featured aren't able
	// to handle changing values.
	if err := upstart.RestartJob(ctx, "featured"); err != nil {
		s.Fatal("Failed to restart featured: ", err)
	}

	// TODO(b/274490519): This *might* race -- See featured.LatePlatformFeatures

	// Right now featured sets things up when we login; so make it happen.
	arg := chrome.EnableFeatures(enabledFeature)
	cr, err := chrome.New(ctx, arg)
	if err != nil {
		s.Fatal("Failed to create chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	data, err := os.ReadFile("/sys/kernel/mm/lru_gen/min_ttl_ms")
	if err != nil {
		s.Fatal("Failed to read min_ttl_ms: ", err)
	}
	minTTLMs := strings.TrimSpace(string(data))

	if minTTLMs != expectedLruGenMinTTLMs {
		s.Fatalf("Wrong min_ttl_ms; expected %q, got %q", expectedLruGenMinTTLMs, data)
	}
}
