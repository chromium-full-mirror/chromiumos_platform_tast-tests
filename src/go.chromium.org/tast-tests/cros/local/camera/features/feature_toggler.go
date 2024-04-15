// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package features provides utilities for loading and parsing on-device feature
// config files.
package features

import (
	"context"
	"os"

	"go.chromium.org/tast/core/errors"
)

var featureDescs = map[string]struct {
	featureToggleKey string
	overrideFilePath string
}{
	HDRnet:        {"hdrnet_enable", "/run/camera/hdrnet_config.json"},
	GcamAE:        {"gcam_ae_enable", "/run/camera/gcam_ae_config.json"},
	FaceDetection: {"face_detection_enable", "/run/camera/face_detection_config.json"},
}

// FeatureToggleConf specifies on or off of a feature with a feature name.
type FeatureToggleConf map[string]bool

// FeatureToggler sets features on or off of a device.
type FeatureToggler struct {
	modelConfig    *ModelConfig
	featureConfigs map[string]FeatureConfig
}

// NewFeatureToggler returns a new FeatureToggler instance.
func NewFeatureToggler(ctx context.Context) (*FeatureToggler, error) {
	f := new(FeatureToggler)
	var err error
	if f.modelConfig, err = ActiveModelConfig(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to get active model config")
	}

	f.featureConfigs = make(map[string]FeatureConfig)
	for _, featureSet := range f.modelConfig.FeatureSet {
		f.featureConfigs[featureSet.Type] = make(FeatureConfig)
		if err := f.modelConfig.FeatureConfig(featureSet.Type, f.featureConfigs[featureSet.Type]); err != nil {
			return nil, errors.Wrapf(err, "failed to get feature config of type %s", featureSet.Type)
		}
	}
	return f, nil
}

func (f *FeatureToggler) removeOverrideFiles() error {
	for featureName := range f.featureConfigs {
		if err := os.RemoveAll(featureDescs[featureName].overrideFilePath); err != nil {
			return errors.Wrapf(err, "failed to remove file %s", featureDescs[featureName].overrideFilePath)
		}
	}
	return nil
}

func (f *FeatureToggler) writeOverrideFiles(ctx context.Context) error {
	if err := f.removeOverrideFiles(); err != nil {
		return errors.Wrap(err, "failed to remove all override files")
	}
	for featureName, featureConfig := range f.featureConfigs {
		if err := WriteFeatureConfig(ctx, featureConfig, featureDescs[featureName].overrideFilePath, true); err != nil {
			return errors.Wrapf(err, "failed to override feature config of %s", featureName)
		}
	}
	return nil
}

// Toggle overwrites camera feature config files. It should be used before a
// camera open device function.
func (f *FeatureToggler) Toggle(ctx context.Context, featureToggleConf FeatureToggleConf) error {
	for featureName, toggleOn := range featureToggleConf {
		featureConfig, ok := f.featureConfigs[featureName]
		if !ok {
			return errors.Errorf("feature %s not in the device", featureName)
		}

		featureDesc, ok := featureDescs[featureName]
		if !ok {
			return errors.Errorf("feature %s does not support toggling", featureName)
		}
		f.featureConfigs[featureName] = MeldFeatureConfig(featureConfig, FeatureConfig{featureDesc.featureToggleKey: toggleOn})
	}

	if err := f.writeOverrideFiles(ctx); err != nil {
		return errors.Wrap(err, "failed to write all override files")
	}
	return nil
}

// CleanUp removes override camera feature config files.
func (f *FeatureToggler) CleanUp() error {
	if err := f.removeOverrideFiles(); err != nil {
		return errors.Wrap(err, "failed to remove all override files")
	}
	return nil
}
