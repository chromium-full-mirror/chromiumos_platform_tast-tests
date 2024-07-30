// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package testutil provides utilities to setup testing environment for camera
// tests.
package testutil

// The list to be used in the hardware dependency field.

// FlakyUSBCamera is a list of flaky USB cameras.
var FlakyUSBCamera = []string{
	// TODO(b/243048705): skip the test on faulty flash. Remove when resolved.
	"0408:3028", "0408:4021", "05c8:03f4",

	// TODO(b/340123520): skip the test on flaky camera. Remove when resolved.
	// We need to skip by VID:PID, because the camera often gets disconnected on use.
	"13d3:56ec",

	// TODO(b/351688757): skip the test on flaky camera. Remove when resolved.
	// We need to skip by VID:PID, because the camera fails to set control/format.
	"0408:302f",

	// TODO(b/282919004): skip the test on flaky camera. Remove when resolved.
	"04ca:7097",
}

// FlakyMIPIModel is a list of models with a flaky MIPI camera.
var FlakyMIPIModel = []string{
	// TODO(b/331445568): Skip until we solve the flakiness. Remove when resolved.
	"homestar",
}

// FlakyUSBModel is a list of models with a flaky USB camera.
var FlakyUSBModel = []string{
	// TODO(b/355089873): skip the test on flaky model. Remove when resolved or better workaround landed.
	"pazquel",

	// TODO(b/346900193): skip the test on flaky model. Remove when resolved.
	// We need to skip by model, because the camera is often completed lost.
	"jelboz360",

	// TODO(b/340123520): skip the test on flaky model. Remove when resolved.
	// We need to skip by model, because the camera is often completed lost.
	"storo", "storo360",

	// TODO(b/351688757): skip the test on flaky model. Remove when resolved.
	// We need to skip by model, because the camera is often completed lost.
	"kracko360", "quandiso360",

	// TODO(b/282919004): skip the test on flaky model. Remove when resolved.
	// We need to skip by model, because the camera is often completed lost.
	"liara",
}

// FlakyModel is a list of models with a flaky camera.
var FlakyModel = append(FlakyMIPIModel, FlakyUSBModel...)
