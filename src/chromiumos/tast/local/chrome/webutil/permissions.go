// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package webutil

import (
	"context"
	"fmt"

	"chromiumos/tast/local/chrome"
	"go.chromium.org/tast/core/errors"
)

// PermissionName represents the name of browser permissions that can be requested by user.
type PermissionName string

// Available permission options.
const (
	PermissionCamera       PermissionName = "camera"
	PermissionMicrophone   PermissionName = "microphone"
	PermissionNotification PermissionName = "notifications"
	PermissionClipboard    PermissionName = "clipboard-write"
)

// PermissionStatus returns the permission status of a certain permission.
// It can be used to decide whether waits for
// It waits for the page load completely before check.
func PermissionStatus(ctx context.Context, conn *chrome.Conn, permissionName PermissionName) (string, error) {
	script := fmt.Sprintf(`navigator.permissions.query({name:%q}).then(res=>res.state)`, permissionName)

	var permissionStatus string
	if err := conn.Eval(ctx, script, &permissionStatus); err != nil {
		return "", errors.Wrapf(err, "failed to check permission %q", permissionName)
	}
	return permissionStatus, nil
}

// IsPermissionPrompt checks whether a certain permission can be prompted on request.
// If a permission is already granted or rejected, it will not be prompted again.
func IsPermissionPrompt(ctx context.Context, conn *chrome.Conn, permissionName PermissionName) (bool, error) {
	permissionStatus, err := PermissionStatus(ctx, conn, permissionName)
	if err != nil {
		return false, err
	}
	return permissionStatus == "prompt", nil
}
