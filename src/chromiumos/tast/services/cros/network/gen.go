// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

//go:generate protoc -I . --go_out=plugins=grpc:../../../../.. bluetooth_network_service.proto
//go:generate protoc -I . --go_out=plugins=grpc:../../../../.. proxy_service.proto
//go:generate protoc -I . --go_out=plugins=grpc:../../../../.. allowlist_service.proto
//go:generate protoc -I . --go_out=plugins=grpc:../../../../.. diag_service.proto
//go:generate protoc -I . --go_out=plugins=grpc:../../../../.. ethernet_service.proto
//go:generate protoc -I . --go_out=plugins=grpc:../../../../.. proxy_setting_service.proto
//go:generate protoc -I . --go_out=plugins=grpc:../../../../.. certificate_service.proto

package network

// Run the following command in CrOS chroot to regenerate protocol buffer bindings:
//
// ~/trunk/src/platform/tast/tools/go.sh generate chromiumos/tast/services/cros/network
