// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package metrics

const tensorflowLiteTitle = "TensorFlow Lite"

// tensorflowLiteCrosboltNameTable maps the PTS description to crosbolt name.
var tensorflowLiteCrosboltNameTable = map[string]string{
	"Model: Mobilenet Quant":     "Mobilenet_Quant",
	"Model: Inception ResNet V2": "Inception_ResNet_V2",
}
