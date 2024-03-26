// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package debug

// Info corresponds to audio_debug_info in cras_types.h.
type Info struct {
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
