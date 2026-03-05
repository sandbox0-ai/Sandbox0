# license-sign

Offline signer for Sandbox0 enterprise license files.

## Usage

```bash
go run ./scripts/license-sign \
  -private-key-file /path/to/license_private.pem \
  -kid s0-2026-01 \
  -subject customer-acme \
  -features multi_cluster \
  -expires-at 2027-03-05T00:00:00Z \
  -out ./license.lic
```

## Flags

- `-private-key-file` (required): Ed25519 private key PEM file.
- `-kid`: key id written into the license envelope.
- `-subject`: customer/license subject identifier.
- `-features`: comma-separated feature list.
- `-not-before`: RFC3339 activation time (default: now).
- `-expires-at`: RFC3339 expiration time.
- `-expires-in`: relative expiration duration from now (used when `-expires-at` is omitted).
- `-out`: output path for signed license file.
