// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package capport holds the CAPPORT-related shared constants and functions used
// in the network package.
package capport

import (
	"context"
	"encoding/json"
	"net/http"
)

const (
	// CapportURL contains the CAPPORT API URL.
	// The value is copied from the example in RFC 8910, that contains a random token and be different for each user.
	CapportURL = "https://www.example.com/captive-portal/api/X54PD39JV"

	// UserPortalURL contains the user portal URL, which is returned by CAPPORT API.
	UserPortalURL = "https://www.example.com/portal.html"
)

// Status is used to represent a CAPPORT status defined at RFC 8908, and be encoded to JSON string.
type Status struct {
	// Captive indicates whether the client is in a state of captivity.
	Captive bool `json:"captive"`

	// UserPortalURL provides the URL of a web portal that MUST be accessed over TLS with which a user can interact.
	UserPortalURL string `json:"user-portal-url"`
}

// HTTPSCapportHandler simulates the CAPPORT HTTPS server that is advertising a portal always closed.
func HTTPSCapportHandler(ctx context.Context, capportURL, userPortalURL string) func(http.ResponseWriter, *http.Request) {
	return func(rw http.ResponseWriter, req *http.Request) {
		switch url := "https://" + req.Host + req.RequestURI; url {
		case capportURL:
			rw.Header().Set("Content-Type", "application/captive+json")
			rw.WriteHeader(http.StatusOK)
			body, _ := json.Marshal(Status{
				true, userPortalURL,
			})
			rw.Write([]byte(body))
		case userPortalURL:
			rw.WriteHeader(http.StatusOK)
			rw.Write([]byte("Portal page"))
		default:
			rw.WriteHeader(http.StatusBadRequest)
		}
	}
}
