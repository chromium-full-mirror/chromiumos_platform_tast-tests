// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package constants contains constant values leveraged by meet tests
package constants

// ConstantServices is a list of services needed by all CFM devices to properly function
var ConstantServices = []string{"cecservice", "dlcservice", "cras", "missived", "hotlined"}
