// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filecheck

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.chromium.org/tast-tests/cros/local/moblab"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// CheckStatefulFiles verifies file permissions on the stateful partition.
// When it returns an error, it's likely because some other test broke the stateful partition isolation.
func CheckStatefulFiles(ctx context.Context, outDir string) []error {
	const (
		root      = "/mnt/stateful_partition"
		errorFile = "stateful_errors.txt"
		maxErrors = 5 // max to print
	)

	// The basic approach here is to specify patterns for paths within a top-level directory, and then add a catch-all
	// Tree pattern that checks anything in the directory that wasn't already explicitly checked or skipped.
	// Any top-level directories not explicitly handled are matched by the final AllPaths pattern.
	patterns := []*Pattern{
		// These only exits in dev images, so they are safe to skip.
		NewPattern(Path("dev_image"), SkipPermCheck(), SkipChildren()),
		NewPattern(Path("dev_image_old"), SkipPermCheck(), SkipChildren()),
		NewPattern(Path("dev_image_new"), SkipPermCheck(), SkipChildren()),
		NewPattern(Path("var_overlay"), SkipPermCheck(), SkipChildren()),

		// These files/directories do not currently pass the general Unix permissions check.
		// TODO(b/306477467) Figure out why these files have relaxed permissions.
		NewPattern(Path("encrypted/var/cache/fwupd/fwupd/fwupdtool"), SkipPermCheck()),
		// The below two files are used by metrics-library to allow all processes to
		// communicate metrics to chrome.
		NewPattern(Path("encrypted/var/lib/metrics/uma-events"), SkipPermCheck()),
		NewPattern(Path("encrypted/var/lib/metrics/uma-events.d"), SkipPermCheck()),
		NewPattern(Path("encrypted/var/lib/metrics/structured/events"), SkipPermCheck()),
		NewPattern(Path("encrypted/var/lib/metrics/structured/keys"), SkipPermCheck()),
		NewPattern(Path("encrypted/var/lib/bluetooth/bluetooth-daemon.current"), SkipPermCheck()),
		NewPattern(Path("encrypted/var/lib/bluetooth/bluetooth-llprivacy.experimental"), SkipPermCheck()),
		NewPattern(Path("encrypted/var/lib/crash_reporter/pending_clean_shutdown"), SkipPermCheck()),
		NewPattern(Path("encrypted/var/lib/featured/store"), SkipPermCheck()),
		NewPattern(Path("encrypted/var/tmp"), SkipPermCheck()),
		NewPattern(Path("encrypted/var/log/asan"), SkipPermCheck()),
		NewPattern(Path("unencrypted/preserve/log/asan"), SkipPermCheck()),
		NewPattern(Path("home/.shadow"), Users("root"), Groups("root"), Mode(0700), SkipPermCheck(), SkipChildren()),

		NewPattern(Root(), Users("root"), Groups("root"), Mode(0755)), // stateful_partition directory itself
		NewPattern(AllPaths()), // everything else not already matched
	}

	// prependPatterns prepends the supplied patterns to the main patterns slice.
	prependPatterns := func(newPatterns ...*Pattern) { patterns = append(newPatterns, patterns...) }

	if moblab.IsMoblab() {
		// On moblab devices, there are additional user dirs and tons of stuff (MySQL, etc.) in /var.
		prependPatterns(
			NewPattern(Tree("home/chronos"), Users("chronos", "root")),
			NewPattern(Tree("home/moblab"), Users("moblab", "root")),
			NewPattern(Tree("var"), SkipChildren()))
	}

	testing.ContextLog(ctx, "Checking ", root)
	problems, numPaths, err := Check(ctx, root, patterns)
	testing.ContextLogf(ctx, "Scanned %d path(s)", numPaths)
	if err != nil {
		return []error{errors.Wrapf(err, "failed to check %v", root)}
	}

	f, err := os.Create(filepath.Join(outDir, errorFile))
	if err != nil {
		return []error{errors.Wrap(err, "failed to create error file")}
	}
	defer f.Close()
	for path, msgs := range problems {
		if _, err := fmt.Fprintf(f, "%v: %v\n", path, strings.Join(msgs, ", ")); err != nil {
			return []error{errors.Wrap(err, "failed to write error file")}
		}
	}

	var result []error
	for path, msgs := range problems {
		if len(result) > maxErrors {
			testing.ContextLogf(ctx, "Too many errors; aborting (see %v)", errorFile)
			break
		}
		result = append(result, errors.Errorf("%v: %v", path, strings.Join(msgs, ", ")))
	}
	return result
}
