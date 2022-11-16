// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package ash implements common libraries used for communication with Chrome Ash.
package ash

// NotificationType describes the types of notifications you can create with chrome.notifications.create()
type NotificationType string

// As defined in https://developer.chrome.com/apps/notifications#type-TemplateType
const (
	NotificationTypeBasic    NotificationType = "basic"
	NotificationTypeImage    NotificationType = "image"
	NotificationTypeList     NotificationType = "list"
	NotificationTypeProgress NotificationType = "progress"
)

// NotificationItem describes an individual item in a list notification.
// As defined in https://developer.chrome.com/docs/extensions/reference/notifications/#type-NotificationItem
type NotificationItem struct {
	Message string `json:"message"`
	Title   string `json:"title"`
}

// Notification corresponds to user notifications in the system tray.
// As defined in https://developer.chrome.com/docs/extensions/reference/notifications/#type-NotificationOptions
type Notification struct {
	ID       string             `json:"id,omitempty"`
	Type     NotificationType   `json:"type"`
	Title    string             `json:"title"`
	Message  string             `json:"message"`
	IconURL  string             `json:"iconUrl"`
	ImageURL string             `json:"imageUrl,omitempty"`
	Priority int                `json:"priority,omitempty"`
	Progress int                `json:"progress,omitempty"`
	Items    []NotificationItem `json:"items,omitempty"`
}

const (
	// DefaultIconURL can be used in creating notifications of any type, as iconURL is always required.
	DefaultIconURL = "data:image/gif;base64,R0lGODlhAQABAIAAAP///wAAACH5BAEAAAAALAAAAAABAAEAAAICRAEAOw=="
	// DefaultImageURL can be used in creating Image type notifications.
	DefaultImageURL = "data:image/gif;base64,iVBORw0KGgoAAAANSUhEUgAAAAoAAAAKCAYAAACNMs+9AAAAFUlEQVR42mNk+P+/noEIwDiqkL4KAbERGO3PogdhAAAAAElFTkSuQmCC"
)
