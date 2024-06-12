// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package testutil provides utilities to setup testing environment for camera
// tests.
package testutil

// The list to be used in the hardware dependency field.

// FlakyUSBCamera is a list of flaky USB cameras.
var FlakyUSBCamera = append(flakyUSBCamera1, flakyUSBCamera2...)

// FlakyMIPIModel is a list of models with a flaky MIPI cameras.
var FlakyMIPIModel = flakyMIPIModel1

// Below is the list of known specific models. Normally we shouldn't need to use them directly.

// TODO(b/331445568): Skip until we solve the flakiness. Remove when resolved.
var flakyMIPIModel1 = []string{"homestar"}

// TODO(b/243048705): skip the test on faulty flash. Remove when resolved.
var flakyUSBCamera1 = []string{"0408:3028", "0408:4021", "05c8:03f4"}

// TODO(b/340123520): skip the test on flaky camera. Remove when resolved.
var flakyUSBCamera2 = []string{"13d3:56ec"}
