// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

//go:generate protoc -I . --go_out=plugins=grpc:../../../../../.. proxy_service.proto
//go:generate protoc -I . --go_out=plugins=grpc:../../../../../.. allowlist_service.proto
//go:generate protoc -I . --go_out=plugins=grpc:../../../../../.. diag_service.proto
//go:generate protoc -I . --go_out=plugins=grpc:../../../../../.. ethernet_service.proto
//go:generate protoc -I . --go_out=plugins=grpc:../../../../../.. proxy_setting_service.proto
//go:generate protoc -I . --go_out=plugins=grpc:../../../../../.. certificate_service.proto
//go:generate protoc -I . --go_out=plugins=grpc:../../../../../.. cros_network_config_service.proto
//go:generate protoc -I . --go_out=plugins=grpc:../../../../../.. test_arc_connectivity_app_service.proto

package network

// Run the following command in CrOS chroot to regenerate protocol buffer bindings:
//
// ~/chromiumos/src/platform/tast/tools/go.sh generate go.chromium.org/tast-tests/cros/services/cros/network
