// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package browser

import (
	"context"

	"chromiumos/tast/common/chrome/ash"
	"go.chromium.org/tast/core/errors"
)

// This file contains types and functions used to create or dismiss notifications from browser via chrome.notification APIs, which is on the sender side of the Chrome notifications.
// If you look for the ones used to communicate with notifications on the receiver side,
// see chromiumos/tast/local/chrome/ash/notification.go instead.

// NotificationType describes the types of notifications you can create with chrome.notifications.create()
type NotificationType = ash.NotificationType

// As defined in https://developer.chrome.com/apps/notifications#type-TemplateType
const (
	NotificationTypeBasic    = ash.NotificationTypeBasic
	NotificationTypeImage    = ash.NotificationTypeImage
	NotificationTypeList     = ash.NotificationTypeList
	NotificationTypeProgress = ash.NotificationTypeProgress
)

// NotificationItem describes an individual item in a list notification.
// As defined in https://developer.chrome.com/docs/extensions/reference/notifications/#type-NotificationItem
type NotificationItem = ash.NotificationItem

// CreateTestNotification creates a notification with a custom title and message.
// iconUrl is a required field to the chrome.notifiations.create() call so a 1px transparent data-url is hardcoded.
// tconn is an arg to be passed in from active browser under test (either ash-chrome or lacros-chrome).
func CreateTestNotification(ctx context.Context, tconn *TestConn, notificationType NotificationType, title, message string) (string, error) {
	var imageURL string
	if notificationType == NotificationTypeImage {
		// Used a transparent solid color image for testing.
		imageURL = "data:image/gif;base64,iVBORw0KGgoAAAANSUhEUgAAAAoAAAAKCAYAAACNMs+9AAAAFUlEQVR42mNk+P+/noEIwDiqkL4KAbERGO3PogdhAAAAAElFTkSuQmCC"
	}

	var items []NotificationItem
	if notificationType == NotificationTypeList {
		// Used 2 mock items for testing.
		items = []NotificationItem{{Message: "item1", Title: "title1"}, {Message: "item2", Title: "title2"}}
	}

	var id string
	if err := tconn.Call(ctx, &id,
		`async (notificationType, title, message, iconUrl, imageURL, items) =>
		tast.promisify(chrome.notifications.create)({
			type: notificationType,
			title: title,
			message: message,
			iconUrl: iconUrl,
			imageUrl: imageURL,
			items: items
		})`,
		notificationType,
		title,
		message,
		"data:image/gif;base64,R0lGODlhAQABAIAAAP///wAAACH5BAEAAAAALAAAAAABAAEAAAICRAEAOw==",
		imageURL,
		items); err != nil {
		return "", errors.Wrap(err, "failed to create notification")
	}
	return id, nil
}

// ClearNotification clear a notification with the given id.
func ClearNotification(ctx context.Context, tconn *TestConn, id string) error {
	if err := tconn.Call(ctx, nil, `tast.promisify(chrome.notifications.clear)`, id); err != nil {
		return errors.Wrap(err, "failed to clear notification")
	}
	return nil
}
