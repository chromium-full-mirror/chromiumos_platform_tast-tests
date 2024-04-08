// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package debug

// Literal values correspond to enum CRAS_IODEV_LAST_OPEN_RESULT in cras_iodev_info.h
const (
	ResultSuccess = "SUCCESS"
	ResultFailure = "FAILURE"
	ResultUnknown = "UNKNOWN"
)

// IodevInfo corresponds to cras_iodev_info in cras_iodev_info.h
type IodevInfo struct {
	Index          uint64 `json:"index"`
	Name           string `json:"name"`
	LastOpenResult string `json:"last_open_result"`
}

// AudioDebugInfo corresponds to audio_debug_info in cras_types.h.
type AudioDebugInfo struct {
	Devices []Device `json:"devices"`
	Streams []Stream `json:"streams"`
}

// Device corresponds to audio_dev_debug_info in cras_types.h.
type Device struct{}

// Stream corresponds to audio_stream_debug_info in cras_types.h
type Stream struct {
	Effects         uint     `json:"effects"`
	ActiveAPEffects []string `json:"active_ap_effects"`
}
