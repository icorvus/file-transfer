# NearDrop

<!-- impeccable:product-schema 1 -->

## Platform
ios

## Stack
Swift and SwiftUI for the iPhone app and Share extension, Go for the Ubuntu PC receiver. Required by the user.

## Users
The owner of an iPhone running iOS 26 and this Ubuntu PC. The user explicitly chose cloud builds and does not want to use the work MacBook. There is no paid Apple Developer Program membership.

## Product Purpose
Select photos in Photos, choose the app in the share sheet, and save the photos directly onto the PC over the local network, with an interaction as easy as AirDrop after initial pairing.

## Operating Context
Both devices must be on a mutually reachable local network. The receiver runs on this PC. No cloud service or account is needed for transfers.

## Capabilities and Constraints
One-time QR pairing, remembered receiver, Bonjour discovery, encrypted uploads, byte-preserving file handling, explicit progress and completion, no overwrites. The working name NearDrop and default destination ~/Pictures/NearDrop are implementation defaults. Compile Swift on a hosted macOS runner. Free signing and installation from the Ubuntu PC are being prepared; no Apple account credentials belong in source or cloud build logs.

## Product Principles
- The share sheet is the primary workflow.
- Pair once, send with one action thereafter.
- Report success only after the receiver confirms saving.
- Preserve the representations supplied by Photos without recompression.
- Keep all transfer traffic on the local network.
