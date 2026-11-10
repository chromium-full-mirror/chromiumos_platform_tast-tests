// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	gotesting "testing"
)

func TestCheckProgramCounter(t *gotesting.T) {
	for _, tc := range []struct {
		name      string
		panicInfo string
		wantErr   bool
	}{
		{
			name:      "no panic data",
			panicInfo: "No panic data\n",
		},
		{
			name:      "cortex-m direct pc",
			panicInfo: "Saved panic data: (NEW)\n=== HANDLER EXCEPTION: 03 === xPSR: 01000000 ===\nr4  : dead6664 r5  : 00000000\npc  : 08012345\n",
		},
		{
			name:      "cortex-m software panic r5 fallback",
			panicInfo: "Saved panic data: (NEW)\n=== PROCESS EXCEPTION: 00 ====== xPSR: ffffffff ===\nr4  : dead6664 r5  : 1009482a\npc  : \n",
		},
		{
			name:      "nds32 ipc",
			panicInfo: "Saved panic data: (NEW)\n=== EXCEP: ITLB miss ===\nIPC  00054321 IPSW 0007000a\n",
		},
		{
			name:      "risc-v nonzero mepc",
			panicInfo: "Saved panic data: (NEW)\n=== EXCEPTION: MCAUSE=2 ===\nS0   00000000 S1   00000000\nMEPC 80081234\n",
		},
		{
			name:      "risc-v software watchdog s1 fallback",
			panicInfo: "Saved panic data: (NEW)\n=== EXCEPTION: MCAUSE=5 ===\nS0   dead6664 S1   800842a4\nMEPC 00000000\n",
		},
		{
			name:      "risc-v zero mepc and zero s1 fails",
			panicInfo: "Saved panic data: (NEW)\n=== EXCEPTION: MCAUSE=5 ===\nS0   dead6664 S1   00000000\nMEPC 00000000\n",
			wantErr:   true,
		},
	} {
		t.Run(tc.name, func(t *gotesting.T) {
			err := checkProgramCounter(context.Background(), tc.panicInfo)
			if (err != nil) != tc.wantErr {
				t.Fatalf("checkProgramCounter() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
