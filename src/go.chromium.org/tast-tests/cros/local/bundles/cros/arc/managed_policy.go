// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"time"

	arcCommon "go.chromium.org/tast-tests/cros/common/arc"
	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/testenv"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     ManagedPolicy,
		Desc:     "Test ARC policies with prod and preprod DMS",
		Contacts: []string{"arc-core@google.com", "jinrongwu@google.com"},
		// ChromeOS > Software > ARC++ > Core
		BugComponent: "b:488493",
		SoftwareDeps: []string{"chrome", "play_store", "gaia"},
		Attr:         []string{"group:external-dependency"},
		VarDeps: []string{
			arcCommon.ManagedDMSAccountPoolVarName,
		},
		Data: []string{"managed_policy.json"},
		Params: []testing.Param{
			{
				Name:              "prod",
				Val:               policy.DMServerProdURL,
				ExtraAttr:         []string{"group:hw_agnostic"},
				ExtraSoftwareDeps: []string{"android_vm"},
				ExtraSearchFlags:  []*testing.StringPair{testenv.SearchFlag(testenv.DMServerProd)},
			},
			{
				Name:              "preprod",
				Val:               policy.DMServerAlphaURL,
				ExtraAttr:         []string{"group:hw_agnostic"},
				ExtraSoftwareDeps: []string{"android_vm"},
				ExtraSearchFlags:  []*testing.StringPair{testenv.SearchFlag(testenv.DMServerAlpha)},
			},
		},
		Timeout: chrome.GAIALoginTimeout + arc.BootTimeout + 3*time.Minute,
	})
}

func ManagedPolicy(ctx context.Context, s *testing.State) {
	creds, err := credconfig.PickRandomCreds(dma.CredsFromPool(arcCommon.ManagedDMSAccountPoolVarName))
	if err != nil {
		s.Fatal("Failed to get login creds: ", err)
	}
	login := chrome.GAIALogin(creds)

	DMServerURL := s.Param().(string)

	cr, err := chrome.New(
		ctx,
		login,
		chrome.ARCSupported(),
		chrome.DMSPolicy(DMServerURL),
		chrome.ExtraArgs(append(
			// to prevent unnecessary sync operations in arc
			arc.DisableSyncFlags(),
			// to enable verbose logging in arc policy bridge
			"--vmodule=arc_policy_bridge=1")...),
	)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return apps.Launch(ctx, tconn, apps.PlayStore.ID)
	}, &testing.PollOptions{Interval: 5 * time.Second, Timeout: 30 * time.Second}); err != nil {
		s.Fatal("Failed to launch Play Store: ", err)
	}

	const (
		policyPath = "data/data/com.google.android.apps.work.clouddpc.arc/files/policies.json"
	)

	androidDataDir, err := arc.AndroidDataDir(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get android-data path: ", err)
	}

	dutPolicyFilePath := filepath.Join(androidDataDir, policyPath)
	dutPolicies, err := getPolicyMap(ctx, dutPolicyFilePath)
	if err != nil {
		s.Fatalf("Failed to get dut policy from %v: %v", dutPolicyFilePath, err)
	}

	existPolicyFilePath := s.DataPath("managed_policy.json")
	existPolicies, err := getPolicyMap(ctx, existPolicyFilePath)
	if err != nil {
		s.Fatalf("Failed to get exist policy from %v: %v", existPolicyFilePath, err)
	}

	// Compare the keys (except guid) and the nested maps one by one.
	// If it fails, it will take log. That's why not using reflect.DeepEqual().
	if err := isMapEqual(ctx, dutPolicies, existPolicies); err != nil {
		s.Fatal("Failed to verify policy: ", err)
	}
}

func getPolicyMap(ctx context.Context, path string) (map[string]interface{}, error) {
	var policFile *os.File
	var err error

	// It takes the DUT a few seconds to load the policy file.
	// Therefore using a poll.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		policFile, err = os.Open(path)
		return err
	}, &testing.PollOptions{Interval: 5 * time.Second, Timeout: 5 * time.Minute}); err != nil {
		return nil, err
	}
	defer policFile.Close()

	// Read the file contents into a byte slice.
	byteValue, err := io.ReadAll(policFile)
	if err != nil {
		return nil, err
	}

	// Unmarshal the JSON data into a map.
	var policies map[string]interface{}
	if err = json.Unmarshal(byteValue, &policies); err != nil {
		return nil, err
	}

	return policies, nil
}

func isMapEqual(ctx context.Context, dutMap, storedMap map[string]interface{}) error {
	for dutKey, dutValue := range dutMap {
		// SKip guid as it is not part of the policy.
		if dutKey == "guid" {
			continue
		}
		storedValue, exists := storedMap[dutKey]
		if !exists {
			return errors.Errorf("There is no key %v in the storedMap", dutKey)
		}

		// caCerts is an array of map which all contains key "X509".
		if dutKey == "caCerts" {
			dutCerts, err := makeCertsArray(dutValue.([]interface{}))
			if err != nil {
				return err
			}
			storedCerts, err := makeCertsArray(storedValue.([]interface{}))
			if err != nil {
				return err
			}
			if err := isEqualCerts(dutCerts, storedCerts); err != nil {
				return err
			}
		} else if reflect.TypeOf(dutValue) == reflect.TypeOf(storedMap) {
			err := isMapEqual(ctx, dutValue.(map[string]interface{}), storedValue.(map[string]interface{}))
			if err != nil {
				return errors.Wrapf(err, "value of key %v does not match, want %v, got %v", dutKey, storedValue, dutValue)
			}
		} else {
			isEqual := reflect.DeepEqual(dutValue, storedValue)
			if !isEqual {
				return errors.Errorf("value of key %v does not match, want %v, got %v", dutKey, storedValue, dutValue)
			}
		}
	}
	if len(dutMap) != len(storedMap) {
		return errors.Errorf("There are some policies not displayed as expected, want %d polices, got %d", len(storedMap), len(dutMap))
	}
	return nil
}

func makeCertsArray(certs []interface{}) ([]string, error) {
	var certsArray []string
	for _, item := range certs {
		certMap, ok := item.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("expect a map cert {X509:...}, got %v", item)
		}
		certsArray = append(certsArray, certMap["X509"].(string))
	}
	return certsArray, nil
}

func isEqualCerts(dutCerts, storedCerts []string) error {
	for _, dutCert := range dutCerts {
		found := false
		for _, storedCert := range storedCerts {
			if dutCert == storedCert {
				found = true
			}
		}
		if !found {
			return errors.New("One of the cert has not been found")
		}
	}

	if len(dutCerts) != len(storedCerts) {
		return errors.Errorf("the number of certs does not match, want %d, got %d", len(storedCerts), len(dutCerts))
	}
	return nil
}
