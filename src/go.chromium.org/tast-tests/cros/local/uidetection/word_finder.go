// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package uidetection

import (
	pb "go.chromium.org/tast-tests/cros/local/uidetection/api"
)

// Word returns a finder for a given word.
func Word(word string, paramList ...TextParam) *Finder {
	textParams := DefaultTextParams()
	for _, param := range paramList {
		param(textParams)
	}
	detectionRequest := &pb.DetectionRequest{
		DetectionRequestType: &pb.DetectionRequest_WordDetectionRequest{
			WordDetectionRequest: &pb.WordDetectionRequest{
				Word:               word,
				RegexMode:          textParams.RegexMode,
				DisableApproxMatch: textParams.DisableApproxMatch,
				MaxEditDistance:    &textParams.MaxEditDistance,
			},
		},
	}
	return newFromRequest(detectionRequest, word)
}
