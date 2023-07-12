// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

//go:generate protoc -I . --go_out=plugins=grpc:../../../../../.. iwlwifi_pci_rescan.proto
//go:generate protoc -I . --go_out=plugins=grpc:../../../../../.. shill_service.proto
//go:generate protoc -I . --go_out=plugins=grpc:../../../../../.. wifi_service.proto
//go:generate protoc -I . --go_out=plugins=grpc:../../../../../.. hpt_service.proto

package wifi

// Run the following command in CrOS chroot to regenerate protocol buffer bindings:
//
// ~/chromiumos/src/platform/tast/tools/go.sh generate go.chromium.org/tast-tests/cros/services/cros/wifi
