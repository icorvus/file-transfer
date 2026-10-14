# NearDrop protocol v1

The receiver serves HTTPS on LAN port 8787 with a persistent P-256 self-signed certificate. The iPhone pins the SHA-256 of the full DER leaf certificate from a QR code displayed locally on the PC. Bonjour never establishes trust. Every request rejects redirects.

Bonjour service: `_neardrop._tcp.local.`, TXT `v=1`, `id=<receiver ID>`. The iPhone resolves matching receiver IDs to private IPv4 addresses. Discovery can be disabled with `--no-mdns`; the saved numeric address remains a fallback. Networks need mutual IP reachability; this does not implement Apple's proprietary AirDrop radio protocol or Wi-Fi Direct.

## Pairing

The loopback control page generates a one-use 256-bit random secret, valid for 10 minutes. A new code invalidates the previous one. The QR has the form:

```text
neardrop://pair?v=1&host=192.168.1.11&port=8787&id=<32 hex>&name=<encoded name>&fp=<64 hex>&code=<64 hex>
```

`POST /v1/pair`, JSON `{ "name": "My iPhone", "code": "..." }`, returns `{ "id", "name", "deviceId", "token", "maxBytes" }`. The secret is compared in constant time. Only hashes of device bearer tokens are stored on the PC. The phone saves its receiver, pin, and token as one item in a shared Keychain access group, accessible after first unlock on this device only. For iloader-signed builds, the shared App Group created by the signer serves as this Keychain group; the installed main bundle ID is resolved at runtime.

Use the displayed QR as a secret: anyone who obtains an unused code can pair during its lifetime. The control page is not exposed to the network. It validates Host and Origin and requires JSON on mutations, with no permissive CORS policy. Local programs running as the same OS user are inside the trust boundary.

## Upload

`GET /v1/status`, authenticated by `Authorization: Bearer <token>`, returns receiver identity and size limit.

`POST /v1/files` streams one file as its raw body with headers:

- `Authorization: Bearer <token>`
- `Content-Type: application/octet-stream`
- `Content-Length: <positive bytes>`
- `X-Transfer-ID: <stable UUID for this file in the current share session>`
- `X-Filename-Base64: <standard base64 of UTF-8 filename>`
- `X-Content-SHA256: <lowercase SHA-256>`

Success is 201 for a new file, 200 for a duplicate transfer. Both return `{ "id", "name", "originalName", "sha256", "bytes", "savedAt", "deviceName", "committed": true }`. The phone verifies ID, bytes, SHA-256, and committed before counting success.

Errors return JSON `{ "error": "human-readable recovery message" }`. Status codes include 400 malformed/interrupted data, 401 revoked/unpaired phone, 403 bad pairing secret, 409 transfer ID reused for different content, 411 missing/empty length, 413 size limit, 422 checksum mismatch, 503 three upload slots already in use, 507 storage failure. The client exposes a user-controlled retry; it does not repeatedly send the whole batch without confirmation.

## Disk integrity and limits

Each upload streams to a 0600 temporary file in the destination. After size/hash validation and fsync, a prepared receipt is atomically persisted in the state directory. A hard link publishes the complete file without overwriting an existing file or symlink. The output directory is synced, then the committed receipt is persisted before acknowledgement. A lost reply or restart is recovered by the same `(device ID, transfer ID)`. A prepared receipt with matching on-disk content can be completed after a crash.

State and receipts are private local files. Do not share them in bug reports. The receiver locks its state directory to prevent two processes using one identity. The upload commit is serialized within a process; up to three bodies can stream concurrently. There is no user-level disk quota or receipt garbage collector yet. Force-killing the receiver can leave hidden `.neardrop-partial-*` files; they are never treated as completed photos and can be removed when the receiver is stopped. Normal cancelled/failed requests remove their partial file.

Idempotency lasts as long as the receipt exists. Deleting a saved file manually does not reset its committed receipt. Re-sharing starts new transfer IDs. If a user moves/deletes receipts or changes the output folder, this history is not a backup or synchronization service.

Local Network usage declarations live in the containing iOS app, as specified in [Apple TN3179](https://developer.apple.com/documentation/technotes/tn3179-understanding-local-network-privacy). The app requests this permission in the foreground before the Share extension is used. No Photos library permission is requested: selected files arrive through `NSItemProvider`.
