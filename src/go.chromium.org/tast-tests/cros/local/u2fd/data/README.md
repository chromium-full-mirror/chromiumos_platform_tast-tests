## Customized WebAuthn Test Site

This directory, `src/go.chromium.org/tast-tests/cros/local/u2fd/data`, contains the source
of the customized WebAuthn test site. Some other packages might symlink their
data subdirectory to this directory so that the Data field in tests could
access the sources.
