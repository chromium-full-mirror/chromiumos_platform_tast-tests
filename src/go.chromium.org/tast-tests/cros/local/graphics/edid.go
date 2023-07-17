// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"encoding/hex"
	"regexp"

	"go.chromium.org/tast/core/errors"
)

// EdidType describes the types like DP or HDMI for Edids
type EdidType int

const (
	// EdidDP type for Edids
	EdidDP EdidType = iota
	// EdidHDMI type for Edids
	EdidHDMI
)

// Edid structure is used to describe the structure of Edid
type Edid struct {
	ID          EdidID   // Unique identifier for the EDID
	Name        string   // Name of the display
	Width       int      // Display width in pixels
	Height      int      // Display height in pixels
	RefreshRate int      // Display refresh rate in Hz
	HDR         bool     // True if the display supports HDR, False otherwise
	Edid        string   // Raw EDID string data
	EdidBytes   []byte   // Raw EDID data as a byte slice
	Type        EdidType // Enum representing the type of EDID (e.g., EdidDP, EdidHDMI)
}

// EdidStringToBytes converts the provided EDID string into an array of bytes
func EdidStringToBytes(edidString string) ([]byte, error) {
	// Prune all whitespace from the EDID string.
	prunedEdidString := regexp.MustCompile(`\s*`).ReplaceAllString(edidString, "")

	// Decode the hex-encoded EDID string into bytes.
	edidBytes, err := hex.DecodeString(prunedEdidString)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode EDID string into bytes")
	}

	// Pad the decoded EDID to 256 bytes.
	if len(edidBytes) < 256 {
		padding := make([]byte, 256-len(edidBytes))
		edidBytes = append(edidBytes, padding...)
	}

	return edidBytes, nil
}
