// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	gotesting "testing"
)

func TestCheckECUARTPowerCycle(t *gotesting.T) {
	for _, tc := range []struct {
		name    string
		ecLog   string
		want    bool
		wantErr bool
	}{
		{
			name: "legacy_ec_s0ix_cycle",
			ecLog: "[11:17:18.017 power state 17 = S0->S0ix, in 0x0007]\r\n" +
				"[11:17:18.019 power state 5 = S0ix, in 0x0007]\r\n" +
				"[11:17:27.138 power state 16 = S0ix->S0, in 0x0007]\r\n" +
				"[11:17:27.141 power state 4 = S0, in 0x0007]\r\n",
			want: true,
		},
		{
			name: "legacy_ec_s3_cycle",
			ecLog: "[1.000 power state 9 = S0->S3, in 0x0003]\n" +
				"[1.002 power state 3 = S3, in 0x0003]\n" +
				"[8.005 power state 8 = S3->S0, in 0x0007]\n" +
				"[8.010 power state 4 = S0, in 0x0007]\n",
			want: true,
		},
		{
			name: "zephyr_dbg_s0ix_cycle",
			ecLog: "[00:10:01.100,000] <dbg> ap_pwrseq: Power state: S0 --> S0S0ix\n" +
				"[00:10:01.102,000] <dbg> ap_pwrseq: Power state: S0S0ix --> S0ix\n" +
				"[00:10:10.200,000] <dbg> ap_pwrseq: Power state: S0ix --> S0ixS0\n" +
				"[00:10:10.205,000] <dbg> ap_pwrseq: Power state: S0ixS0 --> S0\n",
			want: true,
		},
		{
			name:  "still_in_s0ix_not_awake",
			ecLog: "[1.002 power state 5 = S0ix, in 0x0007]\n",
			want:  false,
		},
		{
			name:  "no_power_state_lines",
			ecLog: "HC 0x0123\nUSB PD: SNK Ready\n",
			want:  false,
		},
		{
			name: "unexpected_reset_to_g3",
			ecLog: "[1.002 power state 5 = S0ix, in 0x0007]\n" +
				"[2.000 power state 1 = S5, in 0x0000]\n" +
				"[2.100 power state 0 = G3, in 0x0000]\n" +
				"[5.000 power state 4 = S0, in 0x0007]\n",
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *gotesting.T) {
			got, err := checkECUARTPowerCycle(tc.ecLog)
			if (err != nil) != tc.wantErr {
				t.Fatalf("checkECUARTPowerCycle() err = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("checkECUARTPowerCycle() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSuspendSnapshot(t *gotesting.T) {
	s0ixPre, err := parseSuspendSnapshot("746\n6723419012\n")
	if err != nil {
		t.Fatalf("parseSuspendSnapshot(s0ixPre) failed: %v", err)
	}
	s0ixPost, err := parseSuspendSnapshot("747\n6732548100\n")
	if err != nil {
		t.Fatalf("parseSuspendSnapshot(s0ixPost) failed: %v", err)
	}
	if !didDUTSuspend(s0ixPre, s0ixPost) {
		t.Errorf("didDUTSuspend(s0ix increased) = false, want true")
	}

	// Real SLP_S0# residency failure: kernel froze/resumed (swSuccess +1)
	// while hardware S0ix residency did not advance.
	s0ixNoResidency := suspendSnapshot{swSuccess: 747, hasHWCounter: true, hwCounter: s0ixPre.hwCounter}
	if didDUTSuspend(s0ixPre, s0ixNoResidency) {
		t.Errorf("didDUTSuspend(hwCounter unchanged) = true, want false")
	}

	// Reboot during servod stall resets suspend_stats/success to 0.
	rebootPost := suspendSnapshot{swSuccess: 0, hasHWCounter: true, hwCounter: 0}
	if didDUTSuspend(s0ixPre, rebootPost) {
		t.Errorf("didDUTSuspend(reboot reset to 0) = true, want false")
	}

	// S3 / ARM platform without x86 S0ix debugfs counter.
	s3Pre, err := parseSuspendSnapshot("10\n")
	if err != nil {
		t.Fatalf("parseSuspendSnapshot(s3Pre) failed: %v", err)
	}
	s3Post, err := parseSuspendSnapshot("11\n")
	if err != nil {
		t.Fatalf("parseSuspendSnapshot(s3Post) failed: %v", err)
	}
	if !didDUTSuspend(s3Pre, s3Post) {
		t.Errorf("didDUTSuspend(s3 increased) = false, want true")
	}
}
