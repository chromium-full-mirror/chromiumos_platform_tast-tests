// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package commonautoupdate

// AlternativeRootPartitionMap maps known root disk partitions to their
// alternative partitions.
var AlternativeRootPartitionMap = map[string]string{
	"/dev/nvme0n1p3": "/dev/nvme0n1p5",
	"/dev/nvme0n1p5": "/dev/nvme0n1p3",
	"/dev/mmcblk0p3": "/dev/mmcblk0p5",
	"/dev/mmcblk0p5": "/dev/mmcblk0p3",
	"/dev/mmcblk1p3": "/dev/mmcblk1p5",
	"/dev/mmcblk1p5": "/dev/mmcblk1p3",
	"/dev/mmcblk2p3": "/dev/mmcblk2p5",
	"/dev/mmcblk2p5": "/dev/mmcblk2p3",
	"/dev/sda3":      "/dev/sda5",
	"/dev/sda5":      "/dev/sda3",
}
