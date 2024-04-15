// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package util

import (
	"strings"
	"testing"

	"go.chromium.org/tast-tests/cros/common/avl"
)

func TestAvlInfoFromStorageInfo(t *testing.T) {
	testCases := []struct {
		name        string
		diskType    DiskType
		storageInfo []string
		want        avl.Info
		wantErr     bool
	}{
		{
			name:     "valid UFS",
			diskType: UfsDisk,
			storageInfo: []string{
				"Device: /dev/sda",
				"Vendor: SOME_VENDOR",
				"Model: SOME_MODEL",
				"Firmware: SOME_FW",
				"...",
			},
			want: avl.Info{
				PartModel:     "SOME_VENDOR SOME_MODEL",
				PartFirmware:  "SOME_FW",
				ComponentType: "storage",
			},
		},
		{
			name:     "valid NVMe",
			diskType: NvmeDisk,
			storageInfo: []string{
				"...",
				"=== STAR OF INFORMATION SECTION ===",
				"Model Number:                       SOME_MODEL",
				"Serial Number:                      SOME_SERIAL",
				"Firmware Version:                   SOME_FW",
				"...",
			},
			want: avl.Info{
				PartModel:     "SOME_MODEL",
				PartFirmware:  "SOME_FW",
				ComponentType: "storage",
			},
		},
		{
			name:     "valid eMMC",
			diskType: EmmcDisk,
			storageInfo: []string{
				"...",
				"fwrev                | SOME_FW",
				"hwrev                | SOME_HW",
				"manfid               | SOME_MANFID",
				"name                 | SOME_NAME",
				"...",
			},
			want: avl.Info{
				PartModel:     "SOME_MANFID SOME_NAME",
				PartFirmware:  "SOME_FW",
				ComponentType: "storage",
			},
		},
		{
			name:     "missing field",
			diskType: EmmcDisk,
			storageInfo: []string{
				"...",
				"fwrev                | SOME_FW",
				"hwrev                | SOME_HW",
				"manfid               | SOME_MANFID",
				"(name is intentionally omitted)",
			},
			wantErr: true,
		},
		{
			name:     "invalid",
			diskType: UfsDisk,
			storageInfo: []string{
				"invalid storage_info.txt contents",
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(tt *testing.T) {
			bytes := []byte(strings.Join(tc.storageInfo, "\n"))
			got, gotErr := avlInfoFromStorageInfo(bytes, tc.diskType)
			if gotErr != nil && !tc.wantErr {
				tt.Errorf("avlInfoFromStorageInfo(bytes, %q) returned error, expected no error", tc.diskType)
			}
			if gotErr == nil && tc.wantErr {
				tt.Errorf("avlInfoFromStorageInfo(bytes, %q) returned no error, expected error", tc.diskType)
			}
			if got != tc.want {
				tt.Errorf("avlInfoFromStorageInfo(bytes, %q): got %+v, want %+v", tc.diskType, got, tc.want)
			}
		})
	}
}
