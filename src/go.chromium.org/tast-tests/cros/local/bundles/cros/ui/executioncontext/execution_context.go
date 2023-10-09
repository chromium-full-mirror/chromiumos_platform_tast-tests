// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package executioncontext copies some enum values from
// metrics_proto/execution_context.proto.
package executioncontext

const (
	// The constants below should come from metrics_proto/execution_context.proto. However,
	// because that file is hosted in google3 and manually exported to the
	// Chromium repo, getting a copy of the proto file into the ChromiumOS repo
	// is extremely complicated. Since these numbers can't be changed (see comment at
	// https://source.chromium.org/chromium/chromium/src/+/main:content/public/common/process_type.h),
	// we can save signicant work by just copying them here.

	// BrowserProcess enum value represents the browser process.
	BrowserProcess = 1
	// RendererProcess enum value represents the renderer process.
	RendererProcess = 2
	// GPUProcess enum value represents the GPU process.
	GPUProcess = 3
	// UtilityProcess enum value represents the utility process.
	UtilityProcess = 4
	// NetworkServiceProcess enum value represents the network service process
	// (utility process with a NetworkService subtype).
	NetworkServiceProcess = 10

	// MainThread enum value represents the main (starting) thread in a process.
	MainThread = 1
	// IOThread enum value represents the IO thread.
	IOThread = 6
	// CompositorThread enum value represents the compositor thread.
	CompositorThread = 11
)
