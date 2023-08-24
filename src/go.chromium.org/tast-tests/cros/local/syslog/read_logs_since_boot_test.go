// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package syslog

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"go.chromium.org/tast/core/errors"
)

func returnRFCParsedTime(inputtime string) time.Time {
	t, _ := time.Parse(time.RFC3339, inputtime)
	return t
}

var ErrParsingTimestampMsg = errors.New("parsing time \"\" as \"2006-01-02T15:04:05Z07:00\": cannot parse \"\" as \"2006\"")

func TestParseUpstartLogsRFC(t *testing.T) {
	for _, tc := range []struct {
		name     string
		e        []string
		t        time.Time
		wantLogs []string
		wantErr  error
	}{
		{
			name: "GoodLogs",
			e: []string{
				"2022-09-28T00:12:08.924026Z log before reboot",
				"2022-09-28T00:12:08.927021Z first log after reboot",
				"2022-09-28T00:12:08.974023Z second log after reboot",
			},
			t: returnRFCParsedTime("2022-09-28T00:12:08.926021Z"),
			wantLogs: []string{
				"2022-09-28T00:12:08.927021Z first log after reboot",
				"2022-09-28T00:12:08.974023Z second log after reboot",
			},
			wantErr: nil,
		},
		{
			name: "NullLinesOnly",
			e: []string{
				"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00",
			},
			t:        returnRFCParsedTime("2022-09-28T00:12:08.926021Z"),
			wantLogs: nil,
			wantErr:  ErrParsingTimestampMsg,
		},
		{
			name: "NullLineWithLogs",
			e: []string{
				"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x002022-09-28T00:12:08.924026Z log before reboot",
				"2022-09-28T00:12:08.927021Z first log after reboot",
				"2022-09-28T00:12:08.974023Z second log after reboot",
			},
			t: returnRFCParsedTime("2022-09-28T00:12:08.926021Z"),
			wantLogs: []string{
				"2022-09-28T00:12:08.927021Z first log after reboot",
				"2022-09-28T00:12:08.974023Z second log after reboot",
			},
			wantErr: nil,
		},
		{
			name:     "Empty",
			e:        []string{""},
			t:        returnRFCParsedTime("2022-09-28T00:12:08.926021Z"),
			wantLogs: nil,
			wantErr:  ErrParsingTimestampMsg,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logs, err := getLogsSinceRFCTimestamp(tc.e, tc.t)
			if diff := cmp.Diff(logs, tc.wantLogs); diff != "" {
				t.Errorf("Unexpected logs extracted (-got +want): %s", diff)
			}
			if !errors.Is(err, tc.wantErr) && err.Error() != tc.wantErr.Error() {
				t.Errorf("Unexpected error: \n got %s \n wnt %s", err, tc.wantErr)
			}
		})
	}
}
