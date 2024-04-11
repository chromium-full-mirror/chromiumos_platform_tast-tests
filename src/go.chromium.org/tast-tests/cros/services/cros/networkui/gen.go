// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

//go:generate protoc -I . --go_out=plugins=grpc:../../../../../.. proxy_service.proto
//go:generate protoc -I . --go_out=plugins=grpc:../../../../../.. proxy_setting_service.proto
//go:generate protoc -I . --go_out=plugins=grpc:../../../../../.. certificate_service.proto
//go:generate protoc -I . --go_out=plugins=grpc:../../../../../.. cros_network_config_service.proto

package networkui

// Run the following command in CrOS chroot to regenerate protocol buffer bindings:
//
// ~/chromiumos/src/platform/tast/tools/go.sh generate go.chromium.org/tast-tests/cros/services/cros/networkui
