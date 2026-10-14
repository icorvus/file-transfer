# NearDrop

Send photos from an iPhone's **Photos → Share → NearDrop** straight to an Ubuntu PC on the same local network. The iPhone app and Share extension are Swift; the receiver is Go. No cloud server is involved.

**Status:** the Go receiver is built and tested on Ubuntu. The Swift app and Xcode project are implemented, but an Xcode build and real iPhone test are still required. Do not treat the iPhone implementation as device-verified until the checks in [docs/TESTING.md](docs/TESTING.md) pass.

## Start on the PC

```sh
make build
./bin/neardrop
```

Open **http://127.0.0.1:8788** on the PC. Photos save to **~/Pictures/NearDrop** by default. The transfer port is **8787/TCP**; Bonjour discovery uses **5353/UDP**. The control page is only accessible from this PC.

To install the receiver and start it automatically when you log in:

```sh
bash scripts/install-linux.sh
```

The installer builds a binary, installs a user service and an application-menu shortcut, then enables the service. It needs no root privileges. It uses the Go toolchain specified in `go.mod`; the first build may download it. On this PC, the existing Go executable at `/usr/local/go/bin/go` is detected automatically.

Useful commands:

```sh
systemctl --user status neardrop
journalctl --user -u neardrop -f
systemctl --user restart neardrop
systemctl --user disable --now neardrop
```

Customize a manual launch:

```sh
./bin/neardrop --name 'My PC' --out "$HOME/Pictures/iPhone" --max-mb 2048
```

For an installed service, run `systemctl --user edit neardrop` and use an override:

```ini
[Service]
ExecStart=
ExecStart=%h/.local/bin/neardrop --out %h/Pictures/iPhone
```

Then `systemctl --user daemon-reload && systemctl --user restart neardrop`.

## Install on iPhone without a Mac

Use the [cloud build and Ubuntu installation guide](docs/CLOUD-INSTALL.md). GitHub compiles the app on a macOS runner; iloader signs the resulting IPA locally with a free Apple account and installs it over USB. The Share extension is included. Free signing requires renewal every seven days.

The public source repository is [icorvus/file-transfer](https://github.com/icorvus/file-transfer). Builds are started manually from its Actions tab. No Apple signing credentials are uploaded to the cloud build.

## Optional development install using your own Mac

1. Install **Xcode 26 or newer**, open it once, and install the iOS platform support. The iPhone must run **iOS 26 or newer**.
2. Copy this repository to your Mac, or run `make package` on the PC and copy **bin/NearDrop-iPhone.zip** to the Mac. Unzip it.
3. Open **ios/NearDrop.xcodeproj**. It is already generated; no Homebrew, XcodeGen, CocoaPods, or Swift package dependencies are needed.
4. In Xcode Settings → Accounts, add your Apple account. Use an account and installation allowed on your work Mac.
5. Select the project in Xcode. For **both NearDrop and NearDropShare**, open Signing & Capabilities, choose your team, and leave automatic signing enabled. If necessary, change `NEARDROP_BUNDLE_PREFIX` in `ios/Config.xcconfig` to a unique identifier, e.g. `com.yourname.neardrop`. Keep the shared keychain entitlement in both targets.
6. Connect your iPhone by cable, trust the Mac, and enable Developer Mode on the iPhone when prompted. Select the **NearDrop** scheme and your iPhone as the destination, then press **Run**.
7. Open NearDrop on the iPhone. On the PC control page, select the PC's Wi-Fi/Ethernet address, then **Pair an iPhone**. In the iPhone app, tap **Scan pairing code**, allow Camera and Local Network access, and scan the PC's QR code.
8. In Photos, select a photo → Share → **NearDrop**. If it is not visible, open **More**, then add NearDrop to Favorites. Keep the share sheet open until it confirms saving; then tap Done.

The main app must be opened once for pairing and Local Network permission before using its Share extension. Pairing QR codes expire after ten minutes and work once. Generate a new one if pairing fails after it has been consumed.

A free Apple **Personal Team** can install apps on a personal device, but its provisioning expires after **seven days**, requiring rebuilding/reinstalling. Keychain Sharing is supported for free Apple developer accounts. Paid developer membership offers other distribution options; this project does not require purchasing membership to attempt a local development install. See [Apple's Personal Team limits](https://developer.apple.com/help/account/basics/about-your-developer-account) and [supported iOS capabilities](https://developer.apple.com/help/account/reference/supported-capabilities-ios/).

## Everyday use

Once paired, selecting NearDrop in the share sheet starts sending to the remembered PC automatically. Bonjour rediscovers its address after a network change; when discovery is blocked, NearDrop tries the last stored address. No confirmation on the PC is needed for a paired phone.

- Up to **100 photos/videos per share**, with a configurable **2 GiB per-file** receiver limit.
- Files are copied and uploaded sequentially; file data is never decoded or recompressed by NearDrop.
- Original filenames are retained where the share provider supplies them. Unsafe characters are sanitized; duplicate filenames become `IMG_1234 (2).HEIC`.
- The PC verifies SHA-256 and syncs the file and receipt before reporting success.
- Retry inside the current share session skips acknowledged files and deduplicates a file whose acknowledgement was lost, including after a PC restart.
- Cancelling, dismissing the sheet, locking the phone, or iOS terminating the extension can interrupt an in-progress transfer. **There is no background/resumable queue in this version.** Already confirmed files remain saved. Starting a new share is a new transfer and can create another copy.
- Photos controls the representation it gives a Share extension. Use Photos' share options to choose the desired format. **Live Photo motion components, RAW+JPEG pairs, original edits, and “All Photos Data” packages are not guaranteed**; package representations report an error. NearDrop preserves the bytes it receives, including embedded metadata, but does not clone a Photos library or apply original filesystem timestamps.

## Troubleshooting

- **PC not found:** keep the receiver running and PC awake. Use the same LAN; guest Wi-Fi/client isolation can prevent devices from talking. Check Settings → Apps → NearDrop → Local Network. If the PC's IP changed and Bonjour is blocked, pair again.
- **Several network addresses:** choose the PC's actual Wi-Fi or Ethernet address, not a Docker bridge or VPN. The receiver omits interfaces without a running link, but some virtual interfaces may still appear.
- **Firewall:** if UFW is active, allow transfer/discovery traffic only from your LAN. For a `192.168.1.0/24` network, the commands are `sudo ufw allow from 192.168.1.0/24 to any port 8787 proto tcp` and `sudo ufw allow from 192.168.1.0/24 to any port 5353 proto udp`. Adapt the subnet. The installer does not change firewall settings.
- **Signing/keychain error:** both targets must use the same team, bundle prefix, and shared keychain group. Don't remove the entitlement to get past a signing error: the Share extension needs it to read the pairing.
- **Expired signing:** reinstall from Xcode. A development-signed app is not a permanent App Store installation.
- **“Cannot publish file”:** the destination filesystem must support hard links (the normal Ubuntu home filesystem does). FAT/exFAT and some network mounts are unsupported because they cannot provide the current no-overwrite publication step.
- **Changed certificate:** keep `~/.config/neardrop` when upgrading. Deleting it creates a new identity, requiring pairing again. Use Unpair in the control page to revoke a phone immediately.

## Development

```sh
make test                         # race-enabled Go tests and vet
python3 scripts/generate-xcode-project.py
```

On a Mac:

```sh
xcodebuild -project ios/NearDrop.xcodeproj -scheme NearDrop \
  -sdk iphonesimulator -destination 'generic/platform=iOS Simulator' \
  CODE_SIGNING_ALLOWED=NO build
```

Run the Swift tests with Product → Test in Xcode using an iPhone Simulator. CI definitions are included but have not been run from this workspace. The project generator uses only Python's standard library; regenerate after adding Swift source files. Configuration and signing details can go in gitignored `ios/Local.xcconfig`.

See [protocol and security](docs/PROTOCOL.md) and [device acceptance tests](docs/TESTING.md).
