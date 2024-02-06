// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package util

import (
	"bufio"
	"context"
	"os"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast/core/errors"
)

// ReadFirstLine reads the first line from a file.
// Line feed character will be removed to ease converting the string
// into other types.
func ReadFirstLine(ctx context.Context, filePath string) (string, error) {
	var result string
	err := action.Retry(3, func(ctx context.Context) error {
		file, err := os.Open(filePath)
		if err != nil {
			return err
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		if scanner.Scan() {
			result = scanner.Text()
			return nil
		}
		if err := scanner.Err(); err != nil {
			return err
		}
		return errors.Errorf("found no content in %q", filePath)
	}, 100*time.Millisecond)(ctx)
	return result, err
}

// ReadFloat64 reads a line from a file and converts it into float64.
func ReadFloat64(ctx context.Context, filePath string) (float64, error) {
	str, err := ReadFirstLine(ctx, filePath)
	if err != nil {
		return 0., err
	}
	return strconv.ParseFloat(str, 64)
}

// ReadInt64 reads a line from a file and converts it into int64.
func ReadInt64(ctx context.Context, filePath string) (int64, error) {
	str, err := ReadFirstLine(ctx, filePath)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(str, 10, 64)
}
