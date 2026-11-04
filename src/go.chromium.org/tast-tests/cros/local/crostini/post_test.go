// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crostini

import "testing"

func TestParseJournalCursor(t *testing.T) {
	for _, tc := range []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "multiline with trailing newline",
			output: "Nov 16 00:00:00 penguin systemd[1]: Started foo.\n-- cursor: s=abc;i=1;b=def\n",
			want:   "s=abc;i=1;b=def",
		},
		{
			name:   "single cursor line with CRLF and spaces",
			output: "-- cursor: s=abc;i=1;b=def  \r\n",
			want:   "s=abc;i=1;b=def",
		},
		{
			name:   "cursor marker only in earlier log line",
			output: "Nov 16 00:00:00 penguin app[2]: -- cursor: fake\nNov 16 00:00:01 penguin app[2]: done\n",
			want:   "",
		},
		{
			name:   "no entries output",
			output: "-- No entries --\n",
			want:   "",
		},
		{
			name:   "empty cursor value",
			output: "line1\n-- cursor:   \n",
			want:   "",
		},
		{
			name:   "empty output",
			output: "",
			want:   "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseJournalCursor([]byte(tc.output)); got != tc.want {
				t.Errorf("parseJournalCursor(%q) = %q, want %q", tc.output, got, tc.want)
			}
		})
	}
}
