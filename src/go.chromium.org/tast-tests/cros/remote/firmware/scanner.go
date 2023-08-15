// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"bufio"
	"context"
	"regexp"
	"strings"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// ScanWithoutExpectedSequenceInSource checks whether there are matches for
// the specified targets in the provided source, without expecting them in a
// specific order.
func (h *Helper) ScanWithoutExpectedSequenceInSource(ctx context.Context, source string, targets []string) error {
	var notFound []string
	for _, target := range targets {
		re := regexp.MustCompile(target)
		if match := re.FindStringSubmatch(source); match == nil {
			notFound = append(notFound, target)
		}
	}
	if len(notFound) != 0 {
		return errors.Errorf("failed to find %q from the provided source", strings.Join(notFound, ", "))
	}
	return nil
}

// ScanWithExpectedSequenceInSource scans a source and checks that specific
// targets are found in the same sequence as they are contained in the given
// slice.
func (h *Helper) ScanWithExpectedSequenceInSource(ctx context.Context, source string, targets []string) error {
	scanner := bufio.NewScanner(strings.NewReader(source))
	for scanner.Scan() {
		if len(targets) > 0 {
			if match := regexp.MustCompile(targets[0]).FindStringSubmatch(scanner.Text()); match != nil {
				testing.ContextLogf(ctx, "Found %q", match[0])
				targets = targets[1:]
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return errors.Wrap(err, "failed to run the scanner")
	}
	if len(targets) != 0 {
		return errors.Errorf("search ended because %s was not found in the log", targets[0])
	}
	return nil
}
