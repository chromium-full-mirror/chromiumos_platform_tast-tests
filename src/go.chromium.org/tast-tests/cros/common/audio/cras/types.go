// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cras

import (
	"fmt"
	"strconv"
	"strings"

	audiopb "go.chromium.org/tast-tests/cros/services/cros/audio"
	"go.chromium.org/tast/core/errors"
)

// MarshalNodeType returns the CrasNodeType as a string in the form expected by
// cras.
func MarshalNodeType(nodeType audiopb.CrasNodeType) (string, error) {
	enumName, ok := audiopb.CrasNodeType_name[int32(nodeType.Number())]
	if !ok {
		return "", errors.Errorf("invalid CrasNodeType %q", nodeType)
	}
	const enumPrefix = "CRAS_NODE_TYPE_"
	return strings.TrimPrefix(enumName, enumPrefix), nil
}

// UnmarshalNodeType parses a node type string to its corresponding enum value,
// as used by cras_test_client.
func UnmarshalNodeType(nodeType string) (audiopb.CrasNodeType, error) {
	const enumPrefix = "CRAS_NODE_TYPE_"
	nodeType = enumPrefix + strings.TrimPrefix(nodeType, enumPrefix)
	enumValue, ok := audiopb.CrasNodeType_value[nodeType]
	if !ok {
		return 0, errors.Errorf("invalid CrasNodeType %q", nodeType)
	}
	return audiopb.CrasNodeType(enumValue), nil
}

// MarshalNodeID formats a CrasNodeID into a string in the format
// "<deviceIndex>:<nodeIndex>", as expected by cras_test_client.
func MarshalNodeID(nodeID *audiopb.CrasNodeID) string {
	if nodeID == nil {
		panic("nodeID is nil")
	}
	return fmt.Sprintf("%d:%d", nodeID.GetDeviceIndex(), nodeID.GetNodeIndex())
}

// UnmarshalNodeID parses a node ID string in the format
// "<deviceIndex>:<nodeIndex>", as used by cras_test_client.
func UnmarshalNodeID(nodeID string) (*audiopb.CrasNodeID, error) {
	parts := strings.Split(nodeID, ":")
	if len(parts) != 2 {
		return nil, errors.Errorf("parse node id: invalid node ID %q", nodeID)
	}
	deviceIndex, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, errors.Wrapf(err, "parse node id: invalid device index %q", parts[0])
	}
	nodeIndex, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, errors.Wrapf(err, "parse node id: invalid node index %q", parts[1])
	}
	return &audiopb.CrasNodeID{
		DeviceIndex: int32(deviceIndex),
		NodeIndex:   int32(nodeIndex),
	}, nil
}

// MarshalSampleFormat returns the SampleFormat as a string in the form expected
// by cras.
func MarshalSampleFormat(sampleFormat audiopb.SampleFormat) (string, error) {
	enumName, ok := audiopb.SampleFormat_name[int32(sampleFormat.Number())]
	if !ok {
		return "", errors.Errorf("invalid SampleFormat %q", sampleFormat)
	}
	const enumPrefix = "SAMPLE_FORMAT_"
	return strings.TrimPrefix(enumName, enumPrefix), nil
}

// UnmarshalSampleFormat parses the SampleFormat from a string in the format
// used by cras.
func UnmarshalSampleFormat(sampleFormat string) (audiopb.SampleFormat, error) {
	const enumPrefix = "SAMPLE_FORMAT_"
	sampleFormat = enumPrefix + strings.TrimPrefix(sampleFormat, enumPrefix)
	enumValue, ok := audiopb.SampleFormat_value[sampleFormat]
	if !ok {
		return 0, errors.Errorf("invalid SampleFormat %q", sampleFormat)
	}
	return audiopb.SampleFormat(enumValue), nil
}

// MarshalCaptureEffects returns the capture effects as a combined hex string,
// as expected by cras_tests and cras_test_client CLIs.
func MarshalCaptureEffects(effects ...audiopb.CaptureEffect) (string, error) {
	if len(effects) == 0 {
		return "", errors.New("at least one effect is required")
	}
	combinedEffects := 0
	for _, effect := range effects {
		effectBitmask, err := captureEffectBitmask(effect)
		if err != nil {
			return "", errors.Wrapf(err, "failed to get capture effect bitmask for %q", effect)
		}
		combinedEffects |= effectBitmask
	}
	return fmt.Sprintf("0x%02x", combinedEffects), nil
}

// captureEffectBitmask returns the bitmask used for the CaptureEffect.
//
// Will return an error if given an invalid CaptureEffect. All enum values are
// expected to be supported, so if it is invalid it's likely user error or this
// function/enum needs to be updated to support the new value.
func captureEffectBitmask(effect audiopb.CaptureEffect) (int, error) {
	switch effect {
	case audiopb.CaptureEffect_CAPTURE_EFFECT_AEC:
		return 0x01, nil
	case audiopb.CaptureEffect_CAPTURE_EFFECT_NS:
		return 0x02, nil
	case audiopb.CaptureEffect_CAPTURE_EFFECT_AGC:
		return 0x04, nil
	case audiopb.CaptureEffect_CAPTURE_EFFECT_VAD:
		return 0x08, nil
	case audiopb.CaptureEffect_CAPTURE_EFFECT_AEC_ON_DSP_ALLOWED:
		return 0x10, nil
	case audiopb.CaptureEffect_CAPTURE_EFFECT_NS_ON_DSP_ALLOWED:
		return 0x20, nil
	case audiopb.CaptureEffect_CAPTURE_EFFECT_AGC_ON_DSP_ALLOWED:
		return 0x40, nil
	default:
		return 0, errors.Errorf("invalid CaptureEffect %s", effect)
	}
}
