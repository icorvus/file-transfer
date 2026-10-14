# Cloud build and Ubuntu installation

This route does not use a MacBook or require paid Apple Developer membership. Compilation runs on GitHub's hosted macOS runner. Signing happens locally through iloader, using an Apple account that you enter yourself. Apple credentials never belong in GitHub secrets, source code, issues, or chat for this route.

## Build

In the public [icorvus/file-transfer repository](https://github.com/icorvus/file-transfer), choose Actions → Cloud build → Run workflow. The workflow is manual so ordinary commits don't launch unrequested jobs. It validates the Go receiver, compiles the Swift app and Share extension, runs Swift tests on an iPhone simulator, and builds a device IPA.

Download the `NearDrop-iPhone-unsigned` artifact after the `iphone-device` job succeeds. This job is independent of the simulator tests; downloading the IPA does not mean those tests passed. Extract `NearDrop-unsigned.ipa`. The IPA includes an ad-hoc signature solely to preserve entitlement metadata; it is **not yet installable** until iloader signs it with your Apple account.

Public repositories can use GitHub's standard hosted runners for free. Do not switch this workflow to a paid larger runner. Artifacts expire after three days; download the IPA promptly or run a new build later.

## Install from Ubuntu

1. Install **iloader** from its [official GitHub releases](https://github.com/nab138/iloader/releases) or [iloader.app](https://iloader.app/). Avoid similarly named download sites. The Linux AMD64 `.deb` is suitable for this Ubuntu PC. `usbmuxd` is needed and is already present on this PC.
2. Connect the iPhone by USB, unlock it, and tap **Trust This Computer** if prompted.
3. Open iloader and sign in yourself. You may use a separate Apple account; it does not have to be the iCloud account on the iPhone. The signing tool handles the Apple login and two-factor prompts. Never send the password or verification code to the assistant.
4. Select your iPhone and choose **Import IPA** (or the equivalent custom IPA action), then choose `NearDrop-unsigned.ipa`. **Keep the NearDropShare extension.** NearDrop needs two App IDs: one for the app and one for the extension.
5. On the iPhone, follow the signing trust prompts in Settings → General → VPN & Device Management. Enable Developer Mode in Settings → Privacy & Security if prompted, then reboot as instructed by iOS.
6. Open NearDrop once. Pair by scanning a fresh code from the PC's [local control page](http://127.0.0.1:8788), allowing Camera and Local Network access.
7. Select a photo in Photos → Share → NearDrop. Keep the sheet open until saved. Confirm the file appears in `~/Pictures/NearDrop`.

Free development signing expires after seven days. Re-sign/reinstall the same IPA with iloader before/after expiry. SideStore is an optional later route for on-device refresh, but adds setup and another app; it is not needed for the initial direct iloader installation.

## Shared pairing compatibility

iloader's current signing implementation gives the main app and its extensions a shared App Group named `group.<resigned-main-bundle-ID>`. NearDrop detects this installed group and uses it as a Keychain access group; the token stays in Keychain. Normal Xcode-signed builds use the configured explicit Keychain Sharing entitlement instead. Do not remove the Share extension during signing. Real-device verification is still needed to prove that the tool version and iOS profile grant the expected access.

Source references: [iloader installation and IPA support](https://github.com/nab138/iloader), [its signing implementation](https://github.com/nab138/isideload/blob/main/isideload/src/sideload/sideloader.rs), [Apple's app-group/keychain relationship](https://developer.apple.com/documentation/security/sharing-access-to-keychain-items-among-a-collection-of-apps), and [Apple Personal Team limits](https://developer.apple.com/help/account/basics/about-your-developer-account).
