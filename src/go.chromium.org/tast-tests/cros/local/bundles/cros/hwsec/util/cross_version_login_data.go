// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package util

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path"

	cpb "go.chromium.org/chromiumos/system_api/cryptohome_proto"
	uda "go.chromium.org/chromiumos/system_api/user_data_auth_proto"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/common/hwsec/util"
	"go.chromium.org/tast-tests/cros/common/u2fd"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast-tests/cros/local/input"
	localu2fd "go.chromium.org/tast-tests/cros/local/u2fd"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// VaultKeyInfo contains the information of a vault key
type VaultKeyInfo = util.VaultKeyInfo

// NewVaultKeyInfo creates VaultKeyInfo from password, label and lowEntropy
var NewVaultKeyInfo = util.NewVaultKeyInfo

// VaultFSType indicates the type of the file system used for the user vault.
type VaultFSType = util.VaultFSType

const (
	// NoVaultFS represents the absence of the user vault in the snapshot.
	NoVaultFS VaultFSType = iota
	// ECRYPTFSVaultFS represents the usage of eCryptfs for the user vault.
	ECRYPTFSVaultFS
)

// CrossVersionLoginConfig contains the information for cross-version login
type CrossVersionLoginConfig = util.CrossVersionLoginConfig

// NewChallengeAuthCrossVersionLoginConfig creates cross-version login config from challenge auth config and rsa key
func NewChallengeAuthCrossVersionLoginConfig(authConfig *hwsec.AuthConfig, keyLabel string, rsaKey *rsa.PrivateKey) *CrossVersionLoginConfig {
	config := &CrossVersionLoginConfig{
		AuthConfig: *authConfig,
		KeyLabel:   keyLabel,
		RsaKey:     rsaKey,
	}
	return config
}

// removeAllChildren deletes all files and folders from the specified directory.
func removeAllChildren(dirPath string) error {
	dir, err := ioutil.ReadDir(dirPath)
	if err != nil {
		return errors.Wrap(err, "failed to read dir")
	}
	firstErr := error(nil)
	for _, f := range dir {
		fullPath := path.Join([]string{dirPath, f.Name()}...)
		if err := os.RemoveAll(fullPath); err != nil {
			// Continue even after seeing an error, to at least attempt
			// deleting other files.
			firstErr = errors.Wrapf(err, "failed to remove %s", f)
		}
	}
	return firstErr
}

func createChallengeResponseData(ctx context.Context, lf hwsec.LogFunc, cryptohome *hwsec.CryptohomeClient) (*CrossVersionLoginConfig, error) {
	const (
		dbusName    = "org.chromium.TestingCryptohomeKeyDelegate"
		testUser    = "challenge_response_test@chromium.org"
		keyLabel    = "challenge_response_key_label"
		keySizeBits = 2048
	)
	// Enable all RSASSA algorithms, with SHA-1 at the top, as most real-world
	// smart cards support all of them.
	keyAlgs := []cpb.ChallengeSignatureAlgorithm{
		cpb.ChallengeSignatureAlgorithm_CHALLENGE_RSASSA_PKCS1_V1_5_SHA1,
		cpb.ChallengeSignatureAlgorithm_CHALLENGE_RSASSA_PKCS1_V1_5_SHA256,
		cpb.ChallengeSignatureAlgorithm_CHALLENGE_RSASSA_PKCS1_V1_5_SHA384,
		cpb.ChallengeSignatureAlgorithm_CHALLENGE_RSASSA_PKCS1_V1_5_SHA512,
	}

	randReader := rand.New(rand.NewSource(0 /* seed */))
	rsaKey, err := rsa.GenerateKey(randReader, keySizeBits)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate RSA key")
	}
	pubKeySPKIDER, err := x509.MarshalPKIXPublicKey(&rsaKey.PublicKey)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate SubjectPublicKeyInfo")
	}

	dbusConn, err := dbusutil.SystemBus()
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to system D-Bus bus")
	}
	if _, err := dbusConn.RequestName(dbusName, 0 /* flags */); err != nil {
		return nil, errors.Wrap(err, "failed to request the well-known D-Bus name")
	}
	defer dbusConn.ReleaseName(dbusName)

	keyDelegate, err := hwsec.NewCryptohomeKeyDelegate(
		lf, dbusConn, testUser, keyAlgs, rsaKey, pubKeySPKIDER)
	if err != nil {
		return nil, errors.Wrap(err, "failed to export D-Bus key delegate")
	}
	defer keyDelegate.Close()

	authConfig := hwsec.NewChallengeAuthConfig(testUser, dbusName, keyDelegate.DBusPath, pubKeySPKIDER, keyAlgs)
	// Create the challenge-response protected cryptohome.
	if err := cryptohome.WithAuthSession(ctx, testUser, false /* isEphemeral */, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authID string) error {
		if err := cryptohome.CreatePersistentUser(ctx, authID); err != nil {
			return errors.Wrap(err, "failed to create persistent user")
		}
		// Enforce the usage of ecryptfs, so that we can take a usable snapshot of encrypted files.
		if _, err := cryptohome.PreparePersistentVault(ctx, authID, true /* ecryptfs */); err != nil {
			return errors.Wrap(err, "failed to prepare persistent user")
		}
		if err := cryptohome.AddSmartCardAuthFactor(ctx, authID, keyLabel, authConfig); err != nil {
			return errors.Wrap(err, "failed to add smart card auth factor")
		}
		return nil
	}); err != nil {
		return nil, err
	}

	// It's expected that the ecryptfs was used because we specified `Ecryptfs` in `vaultConfig`.
	ecryptfsExists, err := ecryptfsVaultExists(ctx, cryptohome, testUser)
	if err != nil {
		return nil, errors.Wrap(err, "failed to check ecryptfs presence")
	}
	if !ecryptfsExists {
		return nil, errors.New("no ecryptfs user vault found")
	}

	config := NewChallengeAuthCrossVersionLoginConfig(authConfig, keyLabel, rsaKey)
	config.VaultFSType = ECRYPTFSVaultFS
	return config, nil
}

func addWebAuthnData(ctx context.Context, cr *chrome.Chrome, webauthnURL, password string, config *CrossVersionLoginConfig) error {
	conn, _, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, browser.TypeAsh, webauthnURL)
	if err != nil {
		return errors.Wrap(err, "failed to open the browser")
	}
	defer closeBrowser(ctx)
	defer conn.Close()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get test API connection")
	}

	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get keyboard")
	}
	defer keyboard.Close(ctx)

	authCallback := func(ctx context.Context, ui *uiauto.Context) error {
		// Check if the UI is correct.
		if err := ui.Exists(nodewith.ClassName("LoginPasswordView"))(ctx); err != nil {
			return errors.Wrap(err, "failed to find the password input field")
		}
		// Type password into ChromeOS WebAuthn dialog.
		if err := keyboard.Type(ctx, password+"\n"); err != nil {
			return errors.Wrap(err, "failed to type password into ChromeOS auth dialog")
		}
		return nil
	}

	webauthnCred, err := localu2fd.MakeCredentialInLocalSite(ctx, conn, tconn, u2fd.WebAuthnRegistrationConfig{Uv: "preferred"}, authCallback)
	if err != nil {
		return errors.Wrap(err, "failed to perform WebAuthn MakeCredential")
	}
	config.WebAuthnCred = webauthnCred
	return nil
}

func createPasswordData(ctx context.Context, cryptohome *hwsec.CryptohomeClient, supportsLE bool, webauthnURL string) (*CrossVersionLoginConfig, error) {
	const (
		extraPass  = "extraPass"
		extraLabel = "extraLabel"
		pin        = "123456"
		pinLabel   = "pinLabel"
	)
	// Add the new password login data. Enforce the usage of ecryptfs, so that we can take a usable snapshot of encrypted files.
	cr, err := chrome.New(ctx, chrome.ExtraArgs("--cryptohome-use-old-encryption-for-testing"))
	if err != nil {
		return nil, errors.Wrap(err, "failed to log in by Chrome")
	}
	defer cr.Close(ctx)
	username := cr.Creds().User
	password := cr.Creds().Pass

	reply, err := cryptohome.ListAuthFactors(ctx, username)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list auth factors")
	}
	authFactors := reply.ConfiguredAuthFactors
	if len(authFactors) != 1 {
		return nil, errors.Errorf("expected exactly 1 auth factor, but got %v", authFactors)
	}
	keyLabel := authFactors[0].Label

	authConfig := hwsec.NewPassAuthConfig(username, password)
	config := util.NewPassAuthCrossVersionLoginConfig(authConfig, keyLabel)
	ecryptfsExists, err := ecryptfsVaultExists(ctx, cryptohome, username)
	if err != nil {
		return nil, errors.Wrap(err, "failed to check ecryptfs presence")
	}
	if ecryptfsExists {
		config.VaultFSType = ECRYPTFSVaultFS
	}

	if err := cryptohome.WithAuthSession(ctx, username, false /* isEphemeral */, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authID string) error {
		if _, err := cryptohome.AuthenticateAuthFactor(ctx, authID, keyLabel, password); err != nil {
			return errors.Wrap(err, "failed to authenticate auth factor")
		}
		if err := config.AddVaultKeyData(ctx, cryptohome, authID, NewVaultKeyInfo(extraPass, extraLabel, false)); err != nil {
			return errors.Wrap(err, "failed to add vault key data of password")
		}
		if supportsLE {
			if err := config.AddVaultKeyData(ctx, cryptohome, authID, NewVaultKeyInfo(pin, pinLabel, true)); err != nil {
				return errors.Wrap(err, "failed to add vault key data of pin")
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	if err := addWebAuthnData(ctx, cr, webauthnURL, password, config); err != nil {
		return nil, errors.Wrap(err, "failed to add WebAuthn data to config")
	}

	return config, nil
}

// ecryptfsVaultExists returns whether ecryptfs vault exists for the given user.
func ecryptfsVaultExists(ctx context.Context, cryptohome *hwsec.CryptohomeClient, username string) (bool, error) {
	sanitizedUsername, err := cryptohome.GetSanitizedUsername(ctx, username, false /* useDBus */)
	if err != nil {
		return false, errors.Wrap(err, "failed to sanitize username")
	}
	ecryptfsVaultDir := fmt.Sprintf("/home/.shadow/%s/vault", sanitizedUsername)
	if _, err := os.Stat(ecryptfsVaultDir); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, errors.Wrap(err, "failed to perform stat")
	}
	return true, nil
}

// setInstallAttributes sets the install attributes for preparing install_attributes.pb
func setInstallAttributes(ctx context.Context, cryptohome *hwsec.CryptohomeClient) error {
	for name, value := range InstallAttrsContents {
		if err := cryptohome.InstallAttributesSet(ctx, name, value); err != nil {
			return errors.Wrapf(err, "failed to set install attributes %s to value %s", name, value)
		}
	}
	return nil
}

// PrepareCrossVersionLoginData prepares the login data and config for CrossVersionLogin and saves them to dataPath and configPath respectively
func PrepareCrossVersionLoginData(ctx context.Context, lf hwsec.LogFunc, helper hwsec.CmdHelper, dataPath, configPath, webauthnURL string) (retErr error) {
	daemonController := helper.DaemonController()
	cryptohome := helper.CryptohomeClient()

	var configList []CrossVersionLoginConfig

	defer func() {
		for _, config := range configList {
			username := config.AuthConfig.Username
			if err := cryptohome.UnmountAndRemoveVault(ctx, username); err != nil {
				if retErr == nil {
					retErr = errors.Wrapf(err, "failed to remove user vault for %q", username)
				} else {
					testing.ContextLogf(ctx, "Failed to remove user vaultf for %q: %v", username, err)
				}
			}
		}
	}()

	if err := setInstallAttributes(ctx, cryptohome); err != nil {
		return errors.Wrap(err, "failed to populate install attributes")
	}

	supportsLE, err := cryptohome.SupportsLECredentials(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get supported policies")
	} else if !supportsLE {
		testing.ContextLog(ctx, "Device does not support PinWeaver")
	}

	if err := daemonController.Restart(ctx, hwsec.UIDaemon); err != nil {
		return errors.Wrap(err, "failed to restart UI")
	}
	config, err := createPasswordData(ctx, cryptohome, supportsLE, webauthnURL)
	if err != nil {
		return errors.Wrap(err, "failed to create password data")
	}
	config.InstallAttrs = InstallAttrsContents
	configList = append(configList, *config)

	if err := daemonController.Restart(ctx, hwsec.UIDaemon); err != nil {
		return errors.Wrap(err, "failed to restart UI")
	}
	config, err = createChallengeResponseData(ctx, lf, cryptohome)
	if err != nil {
		return errors.Wrap(err, "failed to create challenge-response data")
	}
	configList = append(configList, *config)

	// Note that if the format of either CrossVersionLoginConfigData or CrossVersionLoginConfig is changed,
	// the hwsec.CrossVersionLogin should be modified and the generated data should be regenerated.
	// Create compressed data for mocking the login data in this version, which will be used in hwsec.CrossVersionLogin.
	if err := helper.SaveLoginData(ctx, dataPath, true /*includeTpm*/); err != nil {
		return errors.Wrap(err, "failed to create cross-version-login data")
	}
	// Create JSON file of CrossVersionLoginConfig object in order to record which login method we needed to test in hwsec.CrossVersionLogin.
	configJSON, err := json.MarshalIndent(configList, "", "  ")
	testing.ContextLog(ctx, "Generated config: ", string(configJSON))
	if err != nil {
		return errors.Wrap(err, "failed to encode the cross-version-login config to json")
	}
	if err := ioutil.WriteFile(configPath, configJSON, 0644); err != nil {
		return errors.Wrapf(err, "failed to write file to %q", configPath)
	}
	return nil
}
