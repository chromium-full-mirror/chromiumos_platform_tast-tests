// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dep

import (
	"go.chromium.org/tast/core/testing/hwdep"
)

// SupportedModels returns hardwareDeps condition with list of all supported models.
func SupportedModels() hwdep.Deps {
	return hwdep.D(hwdep.Model(append(asusModelList, hpModelList...)...))
}

// NotSupportedModels returns hardwareDeps condition with list of all not supported models.
func NotSupportedModels() hwdep.Deps {
	return hwdep.D(hwdep.SkipOnModel(append(asusModelList, hpModelList...)...))
}
