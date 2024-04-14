// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package testenv contains common constants, flags and utilities to be used by tests against external dependencies.
package testenv

import (
	"go.chromium.org/tast/core/testing"
)

// SearchFlagKey is the key for Search Flags to specify external dependency.
const SearchFlagKey = "external_dependency"

// ServiceDepName describes the available external dependency target name.
type ServiceDepName string

// Valid ServiceDepName.
// Note that the dependency target name is composed of:
//
//	"<service>.<environment>", where
//
// <service> is any target service name,
// <environment> could be 'preprod' or any specific env name of their services.
const (
	// Android Authentication
	AndroidAuthPreprod ServiceDepName = "AndroidAuth.preprod"

	// Android Checkin
	AndroidCheckinPreprod ServiceDepName = "AndroidCheckin.preprod"

	// DMServer
	DMServerProd     ServiceDepName = "DMServer.prod"
	DMServerAlpha    ServiceDepName = "DMServer.alpha"
	DMServerAutoPush ServiceDepName = "DMServer.autopush"
	DMServerStaging  ServiceDepName = "DMServer.staging"

	// GAIA
	GAIAProd    ServiceDepName = "GAIA.prod"
	GAIASandbox ServiceDepName = "GAIA.sandbox"

	// GFE
	GFEPreprod ServiceDepName = "GFE.preprod"

	// OnePlatform OAuth service
	OAuthPreprod ServiceDepName = "OAuth.preprod"

	// Play Terms
	// TODO(b/315504831): Add staging dependency of Play ToS when readily accessible.
	PlayTermsProd ServiceDepName = "PlayTerms.prod"
)

// SearchFlag generates a StringPair based on the given ServiceDepName.
func SearchFlag(n ServiceDepName) *testing.StringPair {
	return &testing.StringPair{
		Key:   SearchFlagKey,
		Value: string(n),
	}
}
