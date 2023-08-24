// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

type testParamCacheIntegrity struct {
	vmEnabled bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         CacheIntegrity,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Validates that image has all caches ",
		Contacts: []string{
			"arc-performance@google.com",
			"khmel@chromium.org",
		},
		// ChromeOS > Software > ARC++ > Performance
		BugComponent: "b:168382",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"arc_android_data_cros_access", "chrome"},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container"},
			Val: testParamCacheIntegrity{
				vmEnabled: false,
			},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
			Val: testParamCacheIntegrity{
				vmEnabled: true,
			},
		}},
		Timeout: 5 * time.Minute,
	})
}

func CacheIntegrity(ctx context.Context, s *testing.State) {
	d := s.DUT()

	param := s.Param().(testParamCacheIntegrity)

	// Keep resources needed for failure investigation.
	artifactsDir := filepath.Join(s.OutDir(), "artifacts")
	if err := os.Mkdir(artifactsDir, os.ModePerm); err != nil {
		s.Fatal(errors.Wrap(err, "failed to create artifacts dir"))
	}

	// Connect to the gRPC server on the DUT.
	cl, err := rpc.Dial(ctx, d, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	cacheSources, err := extractCacheSources(ctx, d, artifactsDir, param.vmEnabled)
	if err != nil {
		s.Fatal(errors.Wrap(err, "cache source extraction failed"))
	}

	for _, e := range []struct {
		fileName  string
		minSizeKB int64
		maxSizeKB int64
	}{
		{"packages_cache.xml", 50, 200},
		{"tts_state_cache.dat", 50, 500},
		{"gservices_cache/databases/gservices.db", 25, 100},
		{"gms_core_cache/app_chimera/current_config.fb", 256, 2048},
		{"ureadahead.pack", 256, 1024},
	} {
		verifyFile(s, filepath.Join(cacheSources, e.fileName), e.minSizeKB, e.maxSizeKB)
	}

	verifyAPKsAreSymbolicLinks(s, cacheSources, 8, 8)
}

// extractCacheSources extracts caches from the system image.
func extractCacheSources(ctx context.Context, d *dut.DUT, artifactsDir string, vmEnabled bool) (string, error) {
	testing.ContextLog(ctx, "Extracting cache sources")
	var remoteImg string
	if vmEnabled {
		remoteImg = "/opt/google/vms/android/system.raw.img"
	} else {
		remoteImg = "/opt/google/containers/android/system.raw.img"
	}
	localImg := filepath.Join(artifactsDir, "system.raw.img")
	if err := linuxssh.GetFile(
		ctx, d.Conn(), remoteImg, localImg,
		linuxssh.PreserveSymlinks); err != nil {
		return "", errors.Wrapf(err, "failed to get %q from the device", remoteImg)
	}
	defer os.RemoveAll(localImg)

	if err := testexec.CommandContext(
		ctx,
		"unsquashfs",
		"-no-xattrs",
		"-f",
		"-d", artifactsDir,
		localImg,
		"/system/etc/file_hash_cache",
		"/system/etc/packages_cache.xml",
		"/system/etc/gservices_cache",
		"/system/etc/gms_core_cache/app_chimera",
		"/system/etc/tts_state_cache.dat",
		"/system/etc/ureadahead.pack").Run(testexec.DumpLogOnError); err != nil {
		return "", errors.Wrapf(err, "failed to decompress Squashfs image: %v", localImg)
	}

	etcPath := filepath.Join(artifactsDir, "/system/etc")
	subDir := filepath.Join(artifactsDir, "cacheSources")
	if err := os.Rename(etcPath, subDir); err != nil {
		return "", errors.Wrap(err, "failed to move caches to the target dir")
	}

	if !vmEnabled {
		// Container has pack on the host FS.
		// Ignore error, pack would be checked separately.
		_ = linuxssh.GetFile(
			ctx, d.Conn(),
			"/opt/google/containers/android/ureadahead.pack",
			filepath.Join(subDir, "ureadahead.pack"),
			linuxssh.PreserveSymlinks)
	}

	return subDir, nil
}

// verifyFile verifies that path exists and file size is in the acceptance range.
func verifyFile(s *testing.State, path string, minSizeKB, maxSizeKB int64) {
	statInfo, err := os.Stat(path)
	if err != nil {
		s.Errorf("Failed to check %q: %v", path, err)
		return
	}

	sizeKB := statInfo.Size() / 1024
	if sizeKB < minSizeKB {
		s.Errorf("%q is too small (%d < %d KB)", path, sizeKB, minSizeKB)
	} else if sizeKB > maxSizeKB {
		s.Errorf("%q is too big (%d > %d KB)", path, sizeKB, maxSizeKB)
	}
}

// verifyAPKsAreSymbolicLinks verifies that all apks in the given dir are symbolic links
// and total number of apks in the acceptance range.
func verifyAPKsAreSymbolicLinks(s *testing.State, dir string, minAPKCnt, maxAPKCnt int) {
	apkCount := 0
	if err := filepath.Walk(dir,
		func(scanPath string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if path.Ext(scanPath) == ".apk" {
				apkCount++
				if info.Mode()&os.ModeSymlink == 0 {
					s.Errorf("APK must be symbolic link: %q", scanPath)
				}
			}
			return nil
		}); err != nil {
		s.Errorf("Failed to walk dir %q: %v", dir, err)
	}

	if apkCount == 0 {
		s.Errorf("%q has no apk", dir)
	} else if apkCount < minAPKCnt {
		s.Errorf("%q has too little apks (%d < %d)", dir, apkCount, minAPKCnt)
	} else if apkCount > maxAPKCnt {
		s.Errorf("%q has too many apks (%d > %d)", dir, apkCount, minAPKCnt)
	}
}
