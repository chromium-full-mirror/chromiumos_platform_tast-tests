// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// The protoc commands to generate the go files based on the protos on this
// package are specified below.
//
// Note: The full path is used here to allow for importing other proto files
// within this project.

//go:generate protoc -I ../../../../../.. --go_out=plugins=grpc:../../../../../.. go.chromium.org/tast-tests/cros/services/cros/audio/cras_types.proto
//go:generate protoc -I ../../../../../.. --go_out=plugins=grpc:../../../../../.. go.chromium.org/tast-tests/cros/services/cros/audio/cras_test_service.proto
//go:generate protoc -I ../../../../../.. --go_out=plugins=grpc:../../../../../.. go.chromium.org/tast-tests/cros/services/cros/audio/cras_control_service.proto

package audio

// Run the following command in CrOS chroot to regenerate protocol buffer bindings:
//
// ~/chromiumos/src/platform/tast/tools/go.sh generate go.chromium.org/tast-tests/cros/services/cros/audio
