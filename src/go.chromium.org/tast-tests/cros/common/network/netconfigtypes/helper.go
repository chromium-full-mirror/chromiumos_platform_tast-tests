// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package netconfigtypes

// ConvertActivationStateTypeToShillString converts the ActivateStateType enum to its
// corresponding string value, based on
// third_party/cros_system_api/dbus/shill/dbus-constants.h
func ConvertActivationStateTypeToShillString(activationStateType ActivationStateType) string {
	switch activationStateType {
	case UnknownAST:
		return "unknown"
	case NotActivatedAST:
		return "not-activated"
	case ActivatingAST:
		return "activating"
	case PartiallyActivatedAST:
		return "partially-activated"
	case ActivatedAST:
		return "activated"
	case NoServiceAST:
		return "no-service"
	default:
		return ""
	}
}
