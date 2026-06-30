// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tape

import (
	"context"
	"encoding/json"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

// AuthenticationConfigJSONVar holds the name of the variable which stores
// the authentication config variables for TAPE.
const AuthenticationConfigJSONVar = "tape.authentication_config_json"

// LocalRefreshTokenVar holds the name of the variable which stores the
// local refresh Token for TAPE authentication.
const LocalRefreshTokenVar = "tape.local_refresh_token"

// dutTokenFilePath stores the path to the file on device which holds the authorization code
// which is used to authenticate with TAPE.
const dutTokenFilePath = "/tmp/tape_auth_token.json"

// AuthenticationConfig represents the data that's stored in the AuthenticationConfigJSONVar.
type AuthenticationConfig struct {
	ClientID                     string   `json:"client_id"`
	ClientSecret                 string   `json:"client_secret"`
	DroneServiceAccountLocations []string `json:"drone_service_account_locations"`
}

type tapeBaseFixt struct {
	authenticationConfigJSON string
	localRefreshToken        string
}

// NewBaseTapeFixture creates a base fixture that loads service accounts needed
// by tape from the remote host to the DUT. This fixture must be instantiated
// and extended by any test that uses TAPE. Credentials are refreshed every test
// and a valid token is written to a location on the DUT. TAPE looks for that token
// when making requests to the API.
//
// Note that tests must be < 60 minutes
// in order to use TAPE. Otherwise, when run on the server, the token will expire
// after 60 minutes and not get refreshed.
func NewBaseTapeFixture() *tapeBaseFixt {
	return &tapeBaseFixt{}
}

func (f *tapeBaseFixt) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	if ProvidedAccount.Value() != "" {
		return nil
	}
	// Save off variable data since it can't be accessed in the pre or post test functions.
	if configJSON, ok := s.Var(AuthenticationConfigJSONVar); !ok {
		s.Fatal("Authentication config variable not set")
	} else {
		f.authenticationConfigJSON = configJSON
	}

	f.localRefreshToken, _ = s.Var(LocalRefreshTokenVar)
	if err := writeToken2Device(ctx, *s.DUT(), f.authenticationConfigJSON, f.localRefreshToken); err != nil {
		s.Fatal("Failed to write token to device: ", err)
	}

	return nil
}
func (f *tapeBaseFixt) TearDown(ctx context.Context, s *testing.FixtState) {
	// TODO(b/204845193): workaround as fixture do not recover connection.
	if err := s.DUT().Connect(ctx); err != nil {
		s.Fatal("Failed to reconnect to DUT: ", err)
	}

	// Remove the Token file.
	if ProvidedAccount.Value() == "" {
		if _, err := s.DUT().Conn().CommandContext(ctx, "rm", dutTokenFilePath).Output(); err != nil {
			s.Fatal("Failed to remove Token file: ", err)
		}
	}
}
func (f *tapeBaseFixt) Reset(ctx context.Context) error { return nil }
func (f *tapeBaseFixt) PreTest(ctx context.Context, s *testing.FixtTestState) {
	if ProvidedAccount.Value() != "" {
		return
	}
	if err := writeToken2Device(ctx, *s.DUT(), f.authenticationConfigJSON, f.localRefreshToken); err != nil {
		s.Fatal("Failed to write token to device: ", err)
	}
}
func (f *tapeBaseFixt) PostTest(ctx context.Context, s *testing.FixtTestState) {}

func writeToken2Device(ctx context.Context, dut dut.DUT, authConfig, localRefreshToken string) error {
	// Parse the configuration variable.
	var config AuthenticationConfig
	if err := json.Unmarshal([]byte(authConfig), &config); err != nil {
		return errors.Wrap(err, "failed to unmarshal config")
	}

	// Clear out the path where the Token will live on the DUT.
	if _, err := dut.Conn().CommandContext(ctx, "rm", "-f", dutTokenFilePath).Output(); err != nil {
		return errors.Wrap(err, "failed to remove existing token file")
	}

	// Get a token source to use for authentication based on the provided information.
	var tokenJSON []byte
	if TapeToken.Value() != "" {
		tokenJSON = []byte(TapeToken.Value())
	} else {
		var tokenSource oauth2.TokenSource
		if localRefreshToken != "" {
			// If any local credentials were provided, write them directly.
			testing.ContextLog(ctx, "Using user credentials for TAPE authentication")
			tokenSource = oauth2.ReuseTokenSource(nil, &jwtToken{
				conf: &oauth2.Config{
					ClientID:     config.ClientID,
					ClientSecret: config.ClientSecret,
					Endpoint:     google.Endpoint,
					RedirectURL:  "urn:ietf:wg:oauth:2.0:oob",
					Scopes:       []string{"openid email"},
				},
				audience: tapeAudience,
				refresh: &oauth2.Token{
					TokenType:    "Bearer",
					RefreshToken: localRefreshToken,
				},
			})
		} else {
			// Find the first available service account on the host.
			var saPath string
			for _, path := range config.DroneServiceAccountLocations {
				if _, err := os.Stat(path); err == nil {
					saPath = path
					break
				}
			}

			if saPath == "" {
				return errors.New("failed to find a service account to use for authentication")
			}

			testing.ContextLogf(ctx, "Using service account credentials for TAPE authentication: %s", saPath)

			// Read the contents of the file.
			sa, err := os.ReadFile(saPath)
			if err != nil {
				return errors.Wrapf(err, "failed to read content of service account located at: %s", saPath)
			}

			tokenSource, err = createTokenSource(ctx, sa)
			if err != nil {
				return errors.Wrapf(err, "failed to create Token source from service account located at: %s", saPath)
			}
		}

		// Make sure the Token source was set.
		if tokenSource == nil {
			errors.New("failed to create a token source for TAPE")
		}

		// Serialize the content.
		t, err := tokenSource.Token()
		if err != nil {
			return errors.Wrap(err, "failed to create a Token for the provided Token source")
		}

		tokenJSON, err = json.Marshal(t)
		if err != nil {
			return errors.Wrap(err, "failed to marshal Token")
		}
	}
	if err := linuxssh.WriteFile(ctx, dut.Conn(), dutTokenFilePath, tokenJSON, 0644); err != nil {
		return errors.Wrap(err, "failed to write local refresh Token to DUT")
	}
	return nil
}
