# Verification and device handoff

## Verified locally on Ubuntu

The Go tests exercise actual server behavior: persistent pairing and identity, single-use/expired codes, revocation, checksum and size checks, incomplete-body cleanup, safe filenames, collision/symlink protection, concurrent duplicate uploads, idempotency across restarts, prepared-receipt recovery, admin Origin/Host protection, and a real pinned TLS upload.

Run `make test`. Linux cannot build UIKit/SwiftUI or run an iPhone simulator. An Xcode project parser and plist validation are useful structural checks, not a replacement for compilation.

## Required Mac and iPhone checks (pending)

1. Build the NearDrop scheme in Xcode 26+ for iOS Simulator. Confirm both the app and NearDropShare build with Swift 6 checks enabled. Run Product → Test; pairing URL and streaming checksum tests must pass.
2. Choose the same signing team for both targets, install on the iOS 26 iPhone, and open NearDrop. Scan a fresh code from the PC. Allow local network access. Confirm a successful check connection.
3. Select one HEIC photo in Photos. Share → NearDrop. Confirm the sheet reports saved and a file appears in `~/Pictures/NearDrop`. Compare the receipt SHA-256 with `sha256sum` on the PC if needed.
4. Send JPEG, PNG, a screenshot, a video, and a mixed batch. Confirm file counts, supplied filenames, rotation, embedded EXIF, and visual quality. Use a small batch first, then 50–100 photos. The app should not decompress full-resolution images in memory.
5. Send two different files with the same name. Both must survive. Disconnect Wi-Fi during a transfer, reconnect, then tap Retry within the same sheet. Confirm completed files are not duplicated.
6. Test dismissing the sheet, pressing Cancel, locking the phone, and killing/restarting the PC receiver. Already confirmed files must remain intact. Unconfirmed files may need a fresh share; background continuation is not implemented.
7. Deny Local Network permission, then enable it in Settings. Deny Camera permission and use the pairing-link fallback. Confirm each route gives an actionable error and can recover.
8. Reboot the PC or restart the user service. Pairing must remain. Change the PC's DHCP address with Bonjour enabled; discovery should find it. Test Bonjour disabled with the original address as fallback.
9. Revoke the phone in the PC control page. Further uploads must fail until a new pairing succeeds. Try a mismatched certificate fingerprint in a test pairing link; credentials must not be sent.
10. Check the iPhone app and sheet with dark appearance, largest accessibility text sizes, VoiceOver, and Reduce Motion. Confirm controls remain reachable and all states readable. Capture actual iPhone/Simulator screenshots; web screenshots are not native evidence.

Record the Mac's `xcodebuild -version`, iPhone model/iOS version, and the first exact error if a check fails. Never include pairing secrets or bearer tokens in shared logs.
