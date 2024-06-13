// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

//go:generate protoc -I . --go_out=plugins=grpc:../../../../../.. manual_control_image_config.proto
//go:generate protoc -I . --python_out=:../../../remote/bundles/cros/camera/data manual_control_image_config.proto

// Package manualcontrol provides all camera manual control related types compiled from protobuf.
package manualcontrol

// Run the following command in CrOS chroot to regenerate protocol buffer bindings:
//
// ~/chromiumos/src/platform/tast/tools/go.sh generate go.chromium.org/tast-tests/cros/common/camera/manualcontrol
