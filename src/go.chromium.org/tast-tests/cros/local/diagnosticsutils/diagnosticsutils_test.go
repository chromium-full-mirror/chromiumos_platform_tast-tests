// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package diagnosticsutils

import (
	"context"
	"testing"

	"go.chromium.org/tast-tests/cros/local/crosconfig"
	"go.chromium.org/tast/core/errors"
)

func TestOptionalCrosConfig(t *testing.T) {
	getCrosConfig = func(context.Context, string, string) (string, error) { return "test", nil }
	if v, _ := GetOptionalCrosConfig(context.TODO(), ""); v == nil || *v != "test" {
		t.Fatal("GetOptionalCrosConfig failed to read file, got:", v)
	}
	getCrosConfig = func(context.Context, string, string) (string, error) { return "", errors.New("test") }
	if _, err := GetOptionalCrosConfig(context.TODO(), ""); err == nil {
		t.Fatal("GetOptionalCrosConfig should return error")
	}
	getCrosConfig = func(context.Context, string, string) (string, error) {
		return "", errors.Wrap(&crosconfig.ErrNotFound{E: errors.New("test")}, "test")
	}
	if _, err := GetOptionalCrosConfig(context.TODO(), ""); err != nil {
		t.Fatal("GetOptionalCrosConfig should not return ErrNotExist")
	}
}

func TestIsCrosConfigTrue(t *testing.T) {
	getCrosConfig = func(context.Context, string, string) (string, error) { return "true", nil }
	if v, _ := IsCrosConfigTrue(context.TODO(), ""); !v {
		t.Fatal("IsCrosConfigTrue should return true")
	}
	getCrosConfig = func(context.Context, string, string) (string, error) { return "false", nil }
	if v, _ := IsCrosConfigTrue(context.TODO(), ""); v {
		t.Fatal("IsCrosConfigTrue should return false")
	}
	getCrosConfig = func(context.Context, string, string) (string, error) { return "", errors.New("test") }
	if _, err := IsCrosConfigTrue(context.TODO(), ""); err == nil {
		t.Fatal("IsCrosConfigTrue should return error")
	}
	getCrosConfig = func(context.Context, string, string) (string, error) {
		return "", errors.Wrap(&crosconfig.ErrNotFound{E: errors.New("test")}, "test")
	}
	if v, err := IsCrosConfigTrue(context.TODO(), ""); v || err != nil {
		t.Fatal("IsCrosConfigTrue should return false and should not return error")
	}
}
