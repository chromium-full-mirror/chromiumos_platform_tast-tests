// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tflite

import (
	"runtime"
)

func localLibraryDirectory() string {
	if runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64" {
		return "/usr/local/lib64/"
	}
	return "/usr/local/lib/"
}

// SampleDelegateLoaderSettings is the settings to load the built-in sample delegate.
var SampleDelegateLoaderSettings = StableDelegateLoaderSettings{
	DelegatePath: localLibraryDirectory() + "libtensorflowlite_cros_sample_delegate.so",
	DelegateName: "cros_sample_delegate",
}

// MtkNeuronDelegateLoaderSettings is the settings to load MtkNeuronDelegate.
var MtkNeuronDelegateLoaderSettings = StableDelegateLoaderSettings{
	DelegatePath: "/usr/lib64/libtensorflowlite_mtk_neuron_delegate.so",
	DelegateName: "mtk_neuron_delegate",
}

// IntelOpenVINODelegateLoaderSettings is the settings to load IntelOpenVINODelegate.
var IntelOpenVINODelegateLoaderSettings = StableDelegateLoaderSettings{
	DelegatePath: "/usr/lib64/libtensorflowlite_intel_openvino_delegate.so",
	DelegateName: "intel_openvino_delegate",
}
