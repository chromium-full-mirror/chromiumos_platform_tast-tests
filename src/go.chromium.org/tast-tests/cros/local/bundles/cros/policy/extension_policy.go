// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast-tests/cros/local/sysutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ExtensionPolicy,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Check if extension policies can be applied",
		BugComponent: "b:1111617",
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"vsavu@google.com", // Test author
		},
		Timeout:      2 * time.Minute,
		SoftwareDeps: []string{"chrome"},
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:hw_agnostic",
		},
		Data: []string{"extension_policy/policy.json", "extension_policy/background.js", "extension_policy/manifest.json", "extension_policy/schema.json"},
		Params: []testing.Param{{
			Val:     browser.TypeAsh,
			Fixture: fixture.FakeDMS,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               browser.TypeLacros,
			Fixture:           fixture.PersistentLacros,
		}},
	})
}

const extensionPolicyDir = "extension_policy"

var extensionPolicyFiles = []string{"background.js", "manifest.json", "schema.json"}

func ExtensionPolicy(ctx context.Context, s *testing.State) {
	browserType := s.Param().(browser.Type)
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Load extension as unpacked.
	extDir, err := ioutil.TempDir("", "policy_test_extension")
	if err != nil {
		s.Fatal("Failed to create temporary directory for test extension: ", err)
	}
	defer os.RemoveAll(extDir)

	if err := os.Chown(extDir, int(sysutil.ChronosUID), int(sysutil.ChronosGID)); err != nil {
		s.Fatal("Failed to chown test extension dir: ", err)
	}

	for _, file := range extensionPolicyFiles {
		source := filepath.Join(extensionPolicyDir, file)
		target := filepath.Join(extDir, file)
		if err := fsutil.CopyFile(s.DataPath(source), target); err != nil {
			s.Fatalf("Failed to copy %q file to %q: %v", file, extDir, err)
		}

		if err := os.Chown(target, int(sysutil.ChronosUID), int(sysutil.ChronosGID)); err != nil {
			s.Fatalf("Failed to chown %q: %v", file, err)
		}
	}

	extID, err := chrome.ComputeExtensionID(extDir)
	if err != nil {
		s.Fatalf("Failed to compute extension ID for %v: %v", extDir, err)
	}

	b, err := ioutil.ReadFile(s.DataPath("extension_policy/policy.json"))
	if err != nil {
		s.Fatal("Failed to load extension_policy.json: ", err)
	}

	// Set the extension policy.
	pb := policy.NewBlob()
	pb.AddExtensionPolicy(extID, json.RawMessage(b))
	if err := fdms.WritePolicyBlob(pb); err != nil {
		s.Fatal("Failed to write policies: ", err)
	}

	// Same as in policy.json.
	providedPolicy := extensionPolicies{
		SensitiveStringPolicy: "secret",
		VisibleStringPolicy:   "notsecret",
	}

	chromeOptions := []chrome.Option{
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fdms.URL),
		chrome.ExtraArgs("--vmodule=*=1"),
		// autotestprivate is not available in lacros. We always need the extension in Ash to read back its policy values.
		chrome.UnpackedExtension(extDir),
	}
	if browserType == browser.TypeLacros {
		chromeOptions = append(chromeOptions, chrome.LacrosUnpackedExtension(extDir))
	}

	cr, br, closeBrowser, err := browserfixt.SetUpWithNewChrome(
		ctx, browserType,
		lacrosfixt.NewConfig(lacrosfixt.KeepAlive(true)),
		chromeOptions...,
	)
	if err != nil {
		s.Fatal("Failed to setup chrome: ", err)
	}
	defer cr.Close(cleanupCtx)
	defer closeBrowser(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get a test api connection: ", err)
	}

	if err := checkPolicies(ctx, tconn, extID, providedPolicy); err != nil {
		s.Error("Failed to check policies in ash: ", err)
	}

	// Connect to the extension and read the set values.
	bgURL := chrome.ExtensionBackgroundPageURL(extID)
	if err := checkExtension(ctx, br, bgURL, providedPolicy); err != nil {
		s.Error("Failed to check extension: ", err)
	}
}

type extensionPolicies struct {
	// Not handling all fields in the schema.
	SensitiveStringPolicy string `json:"SensitiveStringPolicy"`
	VisibleStringPolicy   string `json:"VisibleStringPolicy"`
}

func checkPolicies(ctx context.Context, tconn *chrome.TestConn, extID string, providedPolicy extensionPolicies) error {
	policies, err := policyutil.PoliciesFromDUT(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to read policies from DUT")
	}

	if policies.Extension[extID] == nil {
		return errors.Errorf("could not find entry for extension %q", extID)
	}

	if policies.Extension[extID]["VisibleStringPolicy"] == nil {
		return errors.Errorf("could not find VisibleStringPolicy for extension %q", extID)
	}

	if policies.Extension[extID]["SensitiveStringPolicy"] == nil {
		return errors.Errorf("could not find SensitiveStringPolicy for extension %q", extID)
	}

	var receivedPolicy extensionPolicies
	if err := json.Unmarshal(
		policies.Extension[extID]["VisibleStringPolicy"].ValueJSON,
		&receivedPolicy.VisibleStringPolicy); err != nil {
		return errors.Wrap(err, "failed to parse received visible policies")
	}
	if err := json.Unmarshal(
		policies.Extension[extID]["SensitiveStringPolicy"].ValueJSON,
		&receivedPolicy.SensitiveStringPolicy); err != nil {
		return errors.Wrap(err, "failed to parse received sensitive policies")
	}

	expectedPolicy := providedPolicy
	expectedPolicy.SensitiveStringPolicy = "********" // Sensitive strings are hidden.

	if receivedPolicy != expectedPolicy {
		return errors.Errorf("extension policy dump missmatch: got %v; want %v", receivedPolicy, expectedPolicy)
	}

	return nil
}

func checkExtension(ctx context.Context, br *browser.Browser, bgURL string, providedPolicy extensionPolicies) error {
	conn, err := br.NewConnForTarget(ctx, chrome.MatchTargetURL(bgURL))
	if err != nil {
		return errors.Wrapf(err, "failed to connect to background page at %q", bgURL)
	}
	defer conn.Close()

	if err := chrome.AddTastLibrary(ctx, conn); err != nil {
		return errors.Wrap(err, "failed to add Tast library to extension")
	}

	var val json.RawMessage
	if err := conn.Call(ctx, &val,
		`tast.promisify(tast.bind(chrome.storage.managed, "get"))`,
		[]string{"SensitiveStringPolicy", "VisibleStringPolicy"}); err != nil {

		return errors.Wrap(err, "failed to read managed storage")
	}

	var extensionPolicy extensionPolicies
	if err := json.Unmarshal(val, &extensionPolicy); err != nil {
		return errors.Wrap(err, "failed to parse extension policies")
	}

	if extensionPolicy != providedPolicy {
		return errors.Errorf("extension policy missmatch: got %v; want %v", extensionPolicy, providedPolicy)
	}

	return nil
}
