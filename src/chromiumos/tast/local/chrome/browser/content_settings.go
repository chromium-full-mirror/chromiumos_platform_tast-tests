// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package browser

import (
	"context"
	"fmt"

	"chromiumos/tast/common/action"
	"chromiumos/tast/errors"
)

// ContentSettingType represents the content setting type.
// Reference to https://developer.chrome.com/docs/extensions/reference/contentSettings/#property
type ContentSettingType string

// Available content setting types.
const (
	CameraContentSetting        ContentSettingType = "camera"
	MicrophoneContentSetting    ContentSettingType = "microphone"
	NotificationsContentSetting ContentSettingType = "notifications"
)

// ContentSettingFilterKey represents the content setting filter keys.
type ContentSettingFilterKey string

// Available content setting filter keys.
const (
	ContentSettingFilterKeyPrimaryPattern ContentSettingFilterKey = "primaryPattern"
	ContentSettingFilterKeyPrimaryURL     ContentSettingFilterKey = "primaryUrl"
)

// Typical values of content settings.
const (
	ContentSettingAllow string = "allow"
	ContentSettingBlock string = "block"
	ContentSettingAsk   string = "ask"
)

// SetContentSetting sets site content settings using Chrome API.
// e.g. chrome.contentSettings.camera.set({primaryPattern: '*://meet.google.com/*', setting:'allow'})
// allows Google Meet to use camera device.
func (b *Browser) SetContentSetting(settingType ContentSettingType, filterKey ContentSettingFilterKey, filterValue, settingValue string) action.Action {
	return func(ctx context.Context) error {
		tconn, err := b.TestAPIConn(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to create Test API connection")
		}
		script := fmt.Sprintf(`chrome.contentSettings.%s.set({%s: '%s', setting: '%s'})`, settingType, filterKey, filterValue, settingValue)
		return tconn.Eval(ctx, script, nil)
	}
}

// GrantPermissions grants one or more permissions to given primary URL patterns.
func (b *Browser) GrantPermissions(ctx context.Context, urlPatterns []string, permissionTypes ...ContentSettingType) error {
	for _, urlPattern := range urlPatterns {
		for _, permissionType := range permissionTypes {
			if err := b.SetContentSetting(
				permissionType, ContentSettingFilterKeyPrimaryPattern, urlPattern, ContentSettingAllow,
			)(ctx); err != nil {
				return errors.Wrapf(err, "failed to grant permissions to %s", urlPattern)
			}
		}
	}
	return nil
}
