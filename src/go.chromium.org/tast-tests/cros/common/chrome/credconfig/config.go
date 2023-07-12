// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package credconfig defines structs to hold common creds configurations of chrome.Chrome.
package credconfig

import (
	"math/rand"
	"strings"

	"go.chromium.org/tast/core/errors"
)

// Creds contains credentials to log into a Chrome user session.
type Creds struct {
	// User is the user name of a user account. It is typically an email
	// address (e.g. example@gmail.com).
	User string
	// Pass is the password of a user account.
	Pass string

	// GAIAID is a GAIA ID used on fake logins. If it is empty, an ID is
	// generated from the user name. The field is ignored on other type of
	// logins.
	GAIAID string

	// Contact is an email address of a user who owns a test account.
	// When logging in with a test account, its contact user may be
	// notified of a login attempt and asked for approval.
	Contact string

	// ParentUser is the user name of a parent account. It is used to
	// approve a login attempt when a child account is supervised by a
	// parent account.
	ParentUser string
	// ParentPass is the pass of a parent account. It is used to approve
	// a login attempt when a child account is supervised by a parent
	// account.
	ParentPass string
}

// ParseCreds parses a string containing a list of credentials.
//
// creds is a string containing multiple credentials separated by newlines:
//
//	user1:pass1[:gaiaID1][:contact][:parentUser1][:parentPass1]
//	user2:pass2[:gaiaID2][:contact][:parentUser2][:parentPass2]
//	user3:pass3[:gaiaID3][:contact][:parentUser3][:parentPass3]
//	...
func ParseCreds(credsText string) ([]Creds, error) {
	// Note: Do not include creds in error messages to avoid accidental
	// credential leaks in logs.
	var credsArray []Creds
	for i, line := range strings.Split(credsText, "\n") {
		line = strings.TrimSpace(line)
		if len(line) == 0 || strings.HasPrefix(line, "#") {
			continue
		}
		ps := strings.SplitN(line, ":", 6)
		if len(ps) < 2 {
			return nil, errors.Errorf("failed to parse credential list: line %d: does not contain a colon", i+1)
		}
		creds := Creds{
			User: ps[0],
			Pass: ps[1],
		}
		if len(ps) >= 3 && len(ps[2]) > 0 {
			creds.GAIAID = ps[2]
		}
		if len(ps) >= 4 && len(ps[3]) > 0 {
			creds.Contact = ps[3]
		}
		if len(ps) >= 6 && len(ps[4]) > 0 && len(ps[5]) > 0 {
			creds.ParentUser = ps[4]
			creds.ParentPass = ps[5]
		}
		credsArray = append(credsArray, creds)
	}
	return credsArray, nil
}

// PickRandomCreds randomly picks one credentials from the passed string. For
// the format details refer to the GAIALoginPool option documentation.
func PickRandomCreds(creds string) (Creds, error) {
	var result Creds
	cs, err := ParseCreds(creds)
	if err != nil {
		return result, err
	}
	result = cs[rand.Intn(len(cs))]
	return result, nil
}

// PickNRandomCreds randomly picks n number of credentials from the passed string. For
// the format details refer to the GAIALoginPool option documentation.
func PickNRandomCreds(creds string, n int) ([]Creds, error) {
	cs, err := ParseCreds(creds)
	if err != nil {
		return nil, err
	}

	if n > len(cs) {
		return nil, errors.Errorf("attempted to pick %d creds when only %d were passed in", n, len(cs))
	}

	rand.Shuffle(len(cs), func(i, j int) { cs[i], cs[j] = cs[j], cs[i] })
	return cs[0:n], nil
}
