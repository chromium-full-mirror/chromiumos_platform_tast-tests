# Data files used by Security tast tests

These files are prefixed with the test name in snake_case.

## OpenSSLBlocklist

OpenSSL needs test certificates which can be time consuming to generate, so
they are pregenerated. Since crypto algorithms are deprecated over time in
favor of stronger algorithms the process for generating the files is documented
here so the algorithms can be updated as needed and new files generated.

These files were generated with the following commands:

* openssl_blocklist_bogus_blocklist
```bash
head -c 16 /dev/urandom | sha256sum
```

* openssl_blocklist_ca.key and openssl_blocklist_cert.key
```bash
openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:4096 \
  -out openssl_blocklist_ca.key
```

* openssl_blocklist_ca.pem
```bash
openssl req -x509 -new -key openssl_blocklist_ca.key -days 3650 -text \
  -out openssl_blocklist_ca.pem
```

* openssl_blocklist_cert.pem

Note that the common name must be 127.0.0.1 for the test to work.

```bash
openssl req -x509 -new -key openssl_blocklist_cert.key \
  -CA openssl_blocklist_ca.pem -CAkey openssl_blocklist_ca.key \
  -days 3650 -text -out openssl_blocklist_cert.pem
```

* openssl_blocklist_serial_blocklist
```bash
echo "serial $(openssl x509 -in openssl_blocklist_cert.pem -text | \
  grep -A 1 Serial | tail -1 \
  | tr -d ': ')" | tee openssl_blocklist_serial_blocklist
```

* openssl_blocklist_sha256_blocklist
```bash
echo "sha256 $(openssl x509 -in openssl_blocklist_cert.pem -fingerprint \
  -sha256 -noout | cut -f2 -d= | tr -d : \
  | tr A-F a-f)" | tee openssl_blocklist_sha256_blocklist
```
