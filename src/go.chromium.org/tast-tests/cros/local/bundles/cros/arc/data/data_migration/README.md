# arc.DataMigration external link files

This directory contains external link files for the archived
pre-migration home data snapshot used in arc.DataMigration Tast
test.

## How to create archived home data to be used by arc.DataMigration

1. Flash the previous version of ARC++ (e.g. ARC++ P).

2. (only for virtio-blk /data on crosvm disk test cases) Create a large
   file in the stateful partition to adjust the size of the disk image
   so that it would fit in devices with small storage (e.g., 32GB).

3. Sign in with the specified test account (See
   `arc.DataMigration.yaml` for username/password).

4. Wait until ARC++ boots and uninstall all unnecessary apps. For
   managed test cases, make sure that the app to be installed during
   the test is available on the Play Store.

5. (optional) Populate files under `/data` or install apps.

6. (only for ->T+ test cases) On the DUT, enable adb root with
   go/adb-root-on-arcvm-user-builds and run the following commands.
```
# Remove test adb key to work around b/289798262.
adb shell rm /data/misc/adb/adb_temp_keys.xml

# Work around Play Store reauthenticaiton issue (b/285820960).
adb shell settings put secure user_setup_complete 0
adb shell setprop persist.sys.arc.force_reauth 1
```

7. On the DUT, create a `.tbz2` file with the following commands:
```
cd /home/.shadow/<hash>/mount
tar --xattrs --selinux -Scjf /tmp/<dest_file_name>.tbz2 .
```

8. Copy the tbz2 file into
   `gs://chromiumos-test-assets-public/tast/cros/arc/` and add/update
   the `.external` file in this directory.
