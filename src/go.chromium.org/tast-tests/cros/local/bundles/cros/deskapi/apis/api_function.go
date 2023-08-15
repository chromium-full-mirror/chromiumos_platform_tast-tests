// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package apis contains functions that evaluation javascript function to run desk api
package apis

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome/browser"
)

// SetAllDesk sets window to all desks.
func SetAllDesk(ctx context.Context, conn *browser.Conn) error {
	const setAllDeskJS = `new Promise((resolve, reject) => {
		chrome.runtime.sendMessage(
			"kflgdebkpepnpjobkdfeeipcjdahoomc", {
				"messageType": "SetWindowProperties",
				"operands": {
					"allDesks": true
				}
			},
			(response) => {
				if(response.errorMessage) {
					reject(new Error(response.errorMessage));
					return;
				}
				resolve();
			});
		})`
	return conn.Eval(ctx, setAllDeskJS, nil)
}

// LaunchDesk launches a new desk.
func LaunchDesk(ctx context.Context, conn *browser.Conn) (string, error) {
	const launchDeskJS = `new Promise((resolve, reject) => {
		chrome.runtime.sendMessage(
			"kflgdebkpepnpjobkdfeeipcjdahoomc", {
				"messageType": "LaunchDesk",
				"operands": {
					"deskName": "test"
				}
			},
			(response) => {
				if(response.errorMessage) {
					reject(new Error(response.errorMessage));
					return;
				}
				resolve(response.operands.deskUuid);
			});
		})`
	var deskID string
	err := conn.Eval(ctx, launchDeskJS, &deskID)
	return deskID, err
}

// RemoveDesk removes a desk.
func RemoveDesk(ctx context.Context, conn *browser.Conn, deskID string) error {
	const removeDeskJS = `async (deskId) => {
		await new Promise((resolve, reject) => {
			chrome.runtime.sendMessage(
				"kflgdebkpepnpjobkdfeeipcjdahoomc", {
					"messageType": "RemoveDesk",
					"operands": {
						"deskId": deskId,
						"skipConfirmation": true
					}
				},
				(response) => {
					if (response.errorMessage) {
						reject(new Error(response.errorMessage));
						return;
					}
					resolve();
				});
			});
		}`
	return conn.Call(ctx, nil, removeDeskJS, deskID)
}

// RemoveDeskWithUndo removes a desk with the option to undo the removal.
func RemoveDeskWithUndo(ctx context.Context, conn *browser.Conn, deskID string) error {
	const removeDeskJS = `async (deskId) => {
		await new Promise((resolve, reject) => {
			chrome.runtime.sendMessage(
				"kflgdebkpepnpjobkdfeeipcjdahoomc", {
					"messageType": "RemoveDesk",
					"operands": {
						"deskId": deskId,
						"options": {
							"combineDesks": false,
							"allowUndo": true,
						},
						"skipConfirmation": true
					}
				},
				(response) => {
					if (response.errorMessage) {
						reject(new Error(response.errorMessage));
						return;
					}
					resolve();
				});
			});
		}`
	return conn.Call(ctx, nil, removeDeskJS, deskID)
}

// GetActiveDesk retrieves the current active desk.
func GetActiveDesk(ctx context.Context, conn *browser.Conn) (string, error) {
	const getActiveDeskJS = `new Promise((resolve, reject) => {
		chrome.runtime.sendMessage(
			"kflgdebkpepnpjobkdfeeipcjdahoomc", {
				"messageType": "GetActiveDesk"
			},
			(response) => {
				if (response.errorMessage) {
					reject(new Error(response.errorMessage));
					return;
				}
				resolve(response.operands.deskUuid);
			});
		})`
	var deskID string
	err := conn.Eval(ctx, getActiveDeskJS, &deskID)
	return deskID, err
}

// SwitchDesk switches to a designated desk.
func SwitchDesk(ctx context.Context, conn *browser.Conn, deskID string) error {
	const switchDeskJS = `async (deskId) => {
		await new Promise((resolve, reject) => {
			chrome.runtime.sendMessage(
				"kflgdebkpepnpjobkdfeeipcjdahoomc", {
					"messageType": "SwitchDesk",
					"operands": {
						"deskId": deskId
					}
				},
				(response) => {
					if(response.errorMessage) {
						reject(new Error(response.errorMessage));
						return;
					}
					resolve();
				});
			});
		}`
	return conn.Call(ctx, nil, switchDeskJS, deskID)
}
