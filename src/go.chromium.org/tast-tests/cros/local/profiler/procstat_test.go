// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package profiler

import (
	"testing"
)

func TestParseProcStat(t *testing.T) {
	table := []struct {
		input string
		user  int64
		sys   int64
	}{
		{
			// According to proc(7),
			// user and sys time are the 14th and 15th fields in
			// /proc/[pid]/stat:
			//   (14) utime  %lu
			//   (15) stime  %lu
			`1 (cat) R 4 5 6 7 8 9 10 11 12 13 14 15 16 17`,
			14,
			15,
		},
		{
			// weird program with spaces and parentheses
			`1 (x _) y) R 4 5 6 7 8 9 10 11 12 13 14 15 16 17`,
			14,
			15,
		},
	}

	for _, item := range table {
		t.Run(item.input, func(t *testing.T) {
			user, sys, err := parseProcStat(item.input)
			if err != nil {
				t.Errorf("failed unexpectedly: %s", err)
			}
			if user != item.user {
				t.Errorf("unexpeced user: want %d; got %d", item.user, user)
			}
			if sys != item.sys {
				t.Errorf("unexpected sys: want %d; got %d", item.sys, sys)
			}
		})
	}
}

func TestParseProcStateWithError(t *testing.T) {
	table := []struct {
		input     string
		errString string
	}{
		{
			`1 (cat) R 4 5 6 7 8 9 10 11 12 13 14`,
			"too few fields",
		},
		{
			`no closing parentheses :(`,
			"')' not found",
		},
	}

	for _, item := range table {
		t.Run(item.input, func(t *testing.T) {
			_, _, err := parseProcStat(item.input)
			if err == nil {
				t.Fatalf("expected error %s; got nil", item.errString)
			}
			if err.Error() != item.errString {
				t.Fatalf("unexpected error: want %s; got %s", item.errString, err)
			}
		})
	}
}
