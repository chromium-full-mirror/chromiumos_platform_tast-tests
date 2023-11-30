// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package captiveportalconsts holds the captive portal-related shared
// constants and functions used in the network package.
package captiveportalconsts

import (
	"net/http"
)

const (
	// RedirectURL contains the URL used by redirect handlers.
	RedirectURL = "http://www.example.com"
	// HTTPSPortalURL contains the URL to be set in the manager.
	HTTPSPortalURL = "https://www.example.com"
)

// RedirectHandler returns 302 redirection responses with the given URL as the Location header.
func RedirectHandler(url string) func(http.ResponseWriter, *http.Request) {
	return func(rw http.ResponseWriter, req *http.Request) {
		http.Redirect(rw, req, url, http.StatusFound)
	}
}

// OkResponseHandler returns 200 responses with the given body as as the response body.
func OkResponseHandler(body string) func(http.ResponseWriter, *http.Request) {
	return func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
		rw.Write([]byte(body))
	}
}

// NoContentHandler returns 204 responses.
func NoContentHandler(rw http.ResponseWriter, req *http.Request) {
	rw.WriteHeader(http.StatusNoContent)
}

// TempRedirectHandler returns 307 redirection responses with the given URL as the Location header.
func TempRedirectHandler(url string) func(http.ResponseWriter, *http.Request) {
	return func(rw http.ResponseWriter, req *http.Request) {
		http.Redirect(rw, req, url, http.StatusTemporaryRedirect)
	}
}
