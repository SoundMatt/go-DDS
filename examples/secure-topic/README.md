# secure-topic

End-to-end payload protection using the `security` package. A `SecureCodec` wrapper applies a `security.Plugin` at the codec layer so published payloads are sealed before the mock broker routes them and opened on receipt.

**Pattern:** composable security — plug any `security.Plugin` into any `dds.Codec[T]` without changing the transport or topic API.

## Run

```sh
go run .
```

## What it shows

| Concept | Where |
|---|---|
| `security.AESGCMPlugin` | AES-256-GCM: confidentiality + integrity + authenticity |
| `security.HMACPlugin` | HMAC-SHA-256: integrity + authenticity, no confidentiality |
| `security.NewRandomKey` | cryptographically random key generation |
| `SecureCodec[T]` (local) | composing `Codec[T]` + `Plugin` without any glue code |
| Tamper detection | wrong key → `cipher: message authentication failed` |

## Expected output

```
── Scenario 1: AES-256-GCM (confidentiality + integrity) ──
  received: node=node-42 payload="classified telemetry"
  wire bytes are AES-GCM ciphertext — unreadable without the key

── Scenario 2: HMAC-SHA-256 (integrity only) ──
  received: node=node-7 payload="status=ok"
  wire bytes are plaintext JSON + 32-byte HMAC tag appended

── Scenario 3: tamper detection (wrong key) ──
  tamper detected: cipher: message authentication failed
  payload rejected — wrong key cannot decrypt or verify
```

## Using with the RTPS transport

Pass the plugin directly to `rtps.New` to protect every payload at the transport layer — no codec wrapper needed:

```go
import "github.com/SoundMatt/go-DDS/rtps"
import "github.com/SoundMatt/go-DDS/security"

key := security.NewRandomKey(32)
plugin, _ := security.NewAESGCMPlugin(key)
p, _ := rtps.New(dds.Domain(0), rtps.WithSecurity(plugin))
```
