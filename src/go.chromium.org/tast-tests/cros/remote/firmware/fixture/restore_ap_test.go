// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/tast/core/errors"
)

func TestRestoreAPIfOffline(t *testing.T) {
	for _, tc := range []struct {
		name       string
		connected  bool
		restoreErr error
		bootErrs   []error
		wantCalls  []string
		wantErr    bool
	}{
		{name: "connected_dut_left_alone", connected: true, wantCalls: []string{"connected"}},
		{name: "dut_that_boots_not_flashed", wantCalls: []string{"connected", "boot"}},
		{name: "unbootable_dut_restored_and_booted", bootErrs: []error{errors.New("no ssh"), nil}, wantCalls: []string{"connected", "boot", "restore", "boot"}},
		{name: "restore_failure_reported", bootErrs: []error{errors.New("no ssh")}, restoreErr: errors.New("flash failed"), wantCalls: []string{"connected", "boot", "restore"}, wantErr: true},
		{name: "boot_failure_after_restore_reported", bootErrs: []error{errors.New("no ssh"), errors.New("still no ssh")}, wantCalls: []string{"connected", "boot", "restore", "boot"}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			err := restoreAPIfOffline(t.Context(),
				func(context.Context) bool { calls = append(calls, "connected"); return tc.connected },
				func(context.Context) error { calls = append(calls, "restore"); return tc.restoreErr },
				func(context.Context) error {
					calls = append(calls, "boot")
					if len(tc.bootErrs) == 0 {
						return nil
					}
					err := tc.bootErrs[0]
					tc.bootErrs = tc.bootErrs[1:]
					return err
				},
			)
			if gotErr := err != nil; gotErr != tc.wantErr {
				t.Errorf("restoreAPIfOffline() = %v, want error %t", err, tc.wantErr)
			}
			if diff := cmp.Diff(tc.wantCalls, calls); diff != "" {
				t.Errorf("restoreAPIfOffline() calls mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
