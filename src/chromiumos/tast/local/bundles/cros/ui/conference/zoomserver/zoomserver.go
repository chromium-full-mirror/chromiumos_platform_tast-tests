// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package zoomserver provides APIs for creating and ending zoom rooms.
package zoomserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"chromiumos/tast/errors"
	"chromiumos/tast/local/bundles/cros/ui/conference"
	"chromiumos/tast/testing"
)

const (
	createRoomAPI = "createaio"
	endRoomAPI    = "endaio"
	// defaultMaxDuration is the default existence time of the room in minutes.
	defaultMaxDuration = 20
)

type responseData struct {
	URL    string `json:"url"`
	RoomID string `json:"room_id"`
	Err    string `json:"err"`
}

// CreateConference creates zoom conference room with specified participants.
// Sends a http request that asks for creating a Zoom conference with specified participants and also
// returns clean up method for closing the opened conference.
// Assume there's a Zoom proxy which can receive http request for creating/closing Zoom conference.
// When Zoom proxy receives "createaio" request, it would create a Zoom conference on specified remote
// server with participants via Chrome Devtools Protocols. And "endaio" means close the conference
// which opened by "createaio".
func CreateConference(ctx context.Context, roomSize int, sessionToken, host string) (room string, cleanup conference.Cleanup, err error) {
	const retryCount = 3
	var data *responseData
	// Create a Zoom conference on remote server dynamically and get conference room
	// link. Retry three times until it successfully gets a conference room link.
	for i := 0; i < retryCount; i++ {
		testing.ContextLogf(ctx, "Attempt #%d to get conference room API", i+1)
		// Set default time for the existence time of the room if there is no deadline.
		maxDuration := defaultMaxDuration
		// Use the remaining time of the case to set the existence time of the room.
		deadline, ok := ctx.Deadline()
		if ok {
			maxDuration = int(math.Ceil(deadline.Sub(time.Now()).Minutes()))
		}
		parameterString := fmt.Sprintf("?count=%d&max_duration=%v", roomSize, maxDuration)
		testing.ContextLogf(ctx, "Create a %d-person zoom room that can exist for %v minutes", roomSize, maxDuration)
		if data, err = sendRequest(ctx, sessionToken, host, createRoomAPI, parameterString); err == nil {
			break
		}
		testing.ContextLog(ctx, "Failed to get conference room: ", err)
	}
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to create multiple participants room")
	}
	// We expect the returned body is a valid url that can be used to issue chatroom request.
	// Check the format.
	room = strings.TrimSpace(string(data.URL))
	if _, err := url.ParseRequestURI(room); err != nil {
		return "", nil, errors.Errorf("returned zoom conference invite link %s is not a valid url", room)
	}

	cleanup = func(ctx context.Context) (err error) {
		_, err = sendRequest(ctx, sessionToken, host, endRoomAPI, "?room_id="+data.RoomID)
		return
	}
	return room, cleanup, nil
}

// sendRequest sends a http request that ask for creating/ending a Zoom conference.
func sendRequest(ctx context.Context, sessionToken, host, api, parameterString string) (*responseData, error) {
	reqURL := fmt.Sprintf("%s/api/room/zoom/%s%s&iszoomcase=true", host, api, parameterString)
	testing.ContextLog(ctx, "Requesting a zoom room from the zoom bot server with request URL: ", reqURL)
	// Create request.
	httpReq, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+sessionToken)
	// Send request.
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	// Handle response.
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("failed to get zoom conference invite link with status %d and body %s", resp.StatusCode, body)
	}

	var data *responseData
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		return nil, err
	}
	if data.Err != "" {
		return nil, errors.New(data.Err)
	}
	return data, nil
}
