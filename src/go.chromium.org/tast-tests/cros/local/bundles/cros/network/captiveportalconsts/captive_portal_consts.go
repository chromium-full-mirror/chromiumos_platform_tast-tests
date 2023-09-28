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

// RedirectHandler is the handler used to redirect when a redirect is found.
func RedirectHandler(url string) func(http.ResponseWriter, *http.Request) {
	return func(rw http.ResponseWriter, req *http.Request) {
		http.Redirect(rw, req, url, http.StatusFound)
	}
}

// RedirectWithNoLocationHandler is the handler used when a portal is suspected.
func RedirectWithNoLocationHandler(rw http.ResponseWriter, req *http.Request) {
	rw.WriteHeader(http.StatusFound)
}

// NoContentHandler is the handler used when the service is online.
func NoContentHandler(rw http.ResponseWriter, req *http.Request) {
	rw.WriteHeader(http.StatusNoContent)
}

// TempRedirectHandler is the handler used when a temporary redirect is found.
func TempRedirectHandler(url string) func(http.ResponseWriter, *http.Request) {
	return func(rw http.ResponseWriter, req *http.Request) {
		http.Redirect(rw, req, url, http.StatusTemporaryRedirect)
	}
}
