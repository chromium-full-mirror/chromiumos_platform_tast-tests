// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package tflite contains helper types and constants to test tflite-related
// components.
package tflite

import (
	"encoding/json"
	"os"

	"go.chromium.org/tast/core/errors"
)

// StableDelegateLoaderSettings should be synced with
// https://github.com/tensorflow/tensorflow/blob/-/tensorflow/lite/acceleration/configuration/configuration.proto
type StableDelegateLoaderSettings struct {
	DelegatePath string `json:"delegate_path"`
	DelegateName string `json:"delegate_name"`
}

// ExecutionPreference should be synced with
// https://github.com/tensorflow/tensorflow/blob/-/tensorflow/lite/acceleration/configuration/configuration.proto
type ExecutionPreference int

// These constants should also be synced with
// https://github.com/tensorflow/tensorflow/blob/-/tensorflow/lite/acceleration/configuration/configuration.proto
const (
	PreferenceUndefined        ExecutionPreference = 0
	PreferenceLowPower         ExecutionPreference = 1
	PreferenceFastSingleAnswer ExecutionPreference = 2
	PreferenceSustainedSpeed   ExecutionPreference = 3
	PreferenceTurboBoost       ExecutionPreference = 4
)

// ExecutionPriority should be synced with
// https://github.com/tensorflow/tensorflow/blob/-/tensorflow/lite/acceleration/configuration/configuration.proto
type ExecutionPriority int

// These constants should also be synced with
// https://github.com/tensorflow/tensorflow/blob/-/tensorflow/lite/acceleration/configuration/configuration.proto
const (
	PriorityUndefined ExecutionPriority = 0
	PriorityLow       ExecutionPriority = 90
	PriorityMedium    ExecutionPriority = 100
	PriorityHigh      ExecutionPriority = 110
)

// OptimizationHint should be synced with
// https://github.com/tensorflow/tensorflow/blob/-/tensorflow/lite/acceleration/configuration/configuration.proto
type OptimizationHint int

// These constants should also be synced with
// https://github.com/tensorflow/tensorflow/blob/-/tensorflow/lite/acceleration/configuration/configuration.proto
const (
	OptimizationNone            OptimizationHint = 0
	OptimizationLowLatency      OptimizationHint = 1
	OptimizationDeepFusion      OptimizationHint = 2
	OptimizationBatchProcessing OptimizationHint = 3
)

// OperationCheckMode should be synced with
// https://github.com/tensorflow/tensorflow/blob/-/tensorflow/lite/acceleration/configuration/configuration.proto
type OperationCheckMode int

// These constants should also be synced with
// https://github.com/tensorflow/tensorflow/blob/-/tensorflow/lite/acceleration/configuration/configuration.proto
const (
	NoOperationCheck      OperationCheckMode = 0
	PerNodeOperationCheck OperationCheckMode = 1
	PreOperationCheck     OperationCheckMode = 2
)

// MtkNeuronSettings should be synced with
// https://github.com/tensorflow/tensorflow/blob/-/tensorflow/lite/acceleration/configuration/configuration.proto
type MtkNeuronSettings struct {
	ExecutionPreference       ExecutionPreference `json:"execution_preference"`
	ExecutionPriority         ExecutionPriority   `json:"execution_priority"`
	OptimizationHints         []OptimizationHint  `json:"optimization_hints"`
	OperationCheckMode        OperationCheckMode  `json:"operation_check_mode"`
	AllowFp16PrecisionForFp32 bool                `json:"allow_fp16_precision_for_fp32"`
	UseAhwb                   bool                `json:"use_ahwb"`
	UseCacheableBuffer        bool                `json:"use_cacheable_buffer"`
	CompileOptions            []string            `json:"compile_options"`
	AcceleratorNames          []string            `json:"accelerator_names"`
	NeuronConfigPath          string              `json:"neuron_config_path"`
	InferenceDeadlineMs       int                 `json:"inference_deadline_ms"`
	InferenceAbortTimeMs      int                 `json:"inference_abort_time_ms"`
}

// StableDelegateSettings is the settings file for creating the stable delegate.
type StableDelegateSettings struct {
	StableDelegateLoaderSettings StableDelegateLoaderSettings `json:"stable_delegate_loader_settings"`
	MtkNeuronSettings            *MtkNeuronSettings           `json:"mtk_neuron_settings,omitempty"`
	// TODO(b/371479305): Add IntelOpenVINOSettings here.
}

// WriteTo will the settings content to the given path.
func (settings StableDelegateSettings) WriteTo(path string) error {
	jsonSettings, err := json.Marshal(settings)
	if err != nil {
		return errors.New("failed to marshal stable delegate settings")
	}
	if err := os.WriteFile(path, jsonSettings, 0644); err != nil {
		return errors.Wrap(err, "failed to write settings.json")
	}
	return nil
}
