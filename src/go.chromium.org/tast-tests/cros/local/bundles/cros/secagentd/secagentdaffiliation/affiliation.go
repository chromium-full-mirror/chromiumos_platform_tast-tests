// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package secagentdaffiliation contains shared helpers for retrieving user's affilliation.
package secagentdaffiliation

import (
	"context"
	"fmt"
	"os"
	"strings"

	"go.chromium.org/tast-tests/cros/local/session"
	"go.chromium.org/tast/core/errors"
)

// GetAffiliationStatus returns if the user is affiliated and the corresponding username (user's name or UUID).
func GetAffiliationStatus(deviceUser, hash string) (bool, string, error) {
	userAffiliatedPath := fmt.Sprintf("/var/lib/secagentd/%s/affiliated", hash)
	buff, err := os.ReadFile(userAffiliatedPath)
	if err != nil {
		if !strings.Contains(err.Error(), "no such file or directory") {
			return false, "", errors.Wrap(err, "failed to read affiliation file")
		}
	} else {
		return true, deviceUser, nil
	}

	userUnaffiliatedPath := fmt.Sprintf("/var/lib/secagentd/%s/unaffiliated", hash)
	buff, err = os.ReadFile(userUnaffiliatedPath)
	if err != nil {
		if !strings.Contains(err.Error(), "no such file or directory") {
			return false, "", errors.Wrap(err, "failed to read unaffiliated file")
		}
	} else {
		return false, string(buff), nil
	}

	return false, "", errors.New("no affiliation files found")
}

// GetAffiliationStatusForSignedInUser returns if the signed in user is affiliated
// and the corresponding username (user's name or UUID).
func GetAffiliationStatusForSignedInUser(ctx context.Context) (bool, string, error) {
	deviceUser, hash, err := GetSignedInUser(ctx)
	if err != nil {
		return false, "", errors.Wrap(err, "failed to get signed in user")
	}
	return GetAffiliationStatus(deviceUser, hash)
}

// GetSignedInUser returns the username and hash of current user.
func GetSignedInUser(ctx context.Context) (string, string, error) {
	sessionManager, err := session.NewSessionManager(ctx)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to start session manager")
	}

	deviceUser, hash, err := sessionManager.RetrievePrimarySession(ctx)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to retrieve primary session")
	}
	return deviceUser, hash, nil
}
