// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package testutil provides utilities to setup testing environment for camera
// tests.
package testutil

// The list to be used in the hardware dependency field.

// FlakyUSBCamera is a list of flaky USB cameras.
var FlakyUSBCamera = append(append(flakyUSBCamera1, flakyUSBCamera2...), flakyUSBCamera3...)

// FlakyMIPIModel is a list of models with a flaky MIPI camera.
var FlakyMIPIModel = flakyMIPIModel1

// FlakyUSBModel is a list of models with a flaky USB camera.
var FlakyUSBModel = append(append(append(flakyUSBModel1, flakyUSBModel2...), flakyUSBModel3...), flakyUSBModel4...)

// FlakyModel is a list of models with a flaky camera.
var FlakyModel = append(FlakyMIPIModel, FlakyUSBModel...)

// Below is the list of known specific models. Normally we shouldn't need to use them directly.

// TODO(b/331445568): Skip until we solve the flakiness. Remove when resolved.
var flakyMIPIModel1 = []string{"homestar"}

// TODO(b/243048705): skip the test on faulty flash. Remove when resolved.
var flakyUSBCamera1 = []string{"0408:3028", "0408:4021", "05c8:03f4"}

// TODO(b/340123520): skip the test on flaky camera. Remove when resolved. (See also flakyUSBModel4)
// We need to skip by VID:PID, because the camera often gets disconnected on use.
var flakyUSBCamera2 = []string{"13d3:56ec"}

// TODO(b/351688757): skip the test on flaky camera. Remove when resolved.
// We need to skip by VID:PID, because the camera fails to set control/format.
var flakyUSBCamera3 = []string{"0408:302f"}

// TODO(b/348997906): skip the test on flaky model. Remove when resolved or better workaround landed.
var flakyUSBModel1 = []string{"pazquel"}

// TODO(b/347640774): skip the test on flaky model. Remove when resolved or better workaround landed.
var flakyUSBModel2 = []string{"beadrix"}

// TODO(b/346900193): skip the test on flaky model. Remove when resolved.
// We need to skip by model, because the camera is often completed lost.
var flakyUSBModel3 = []string{"jelboz360"}

// TODO(b/340123520): skip the test on flaky model. Remove when resolved. (See also flakyUSBCamera2)
// We need to skip by model, because the camera is often completed lost.
var flakyUSBModel4 = []string{"storo", "storo360"}
