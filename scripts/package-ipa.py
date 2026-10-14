#!/usr/bin/env python3
"""Package a device build; this artifact still needs Apple signing to install."""
from pathlib import Path
import plistlib
import shutil
import subprocess
import sys
import tempfile
from zipfile import ZipFile, ZIP_DEFLATED

app, output = map(lambda value: Path(value).resolve(), sys.argv[1:3])
if not app.is_dir() or app.suffix != ".app":
    raise SystemExit("Expected a built iPhone .app directory")
info = plistlib.loads((app / "Info.plist").read_bytes())
if info.get("DTPlatformName") != "iphoneos":
    raise SystemExit("An iPhone device build is required, not a Simulator build")
output.parent.mkdir(parents=True, exist_ok=True)
with tempfile.TemporaryDirectory() as tmp:
    payload = Path(tmp) / "Payload"
    payload.mkdir()
    destination = payload / app.name
    shutil.copytree(app, destination, symlinks=True)
    # Retain intended entitlements in an ad-hoc signature for signing tools.
    # This is NOT a trusted Apple signature and cannot install by itself.
    for bundle in [*destination.glob("PlugIns/*.appex"), destination]:
        bundle_info = plistlib.loads((bundle / "Info.plist").read_bytes())
        entitlements = {
            "application-identifier": "UNSIGNED00." + bundle_info["CFBundleIdentifier"],
            "com.apple.developer.team-identifier": "UNSIGNED00",
            "keychain-access-groups": ["UNSIGNED00.com.wiktor.neardrop.shared"],
            "get-task-allow": True,
        }
        path = Path(tmp) / (bundle.stem + ".plist")
        path.write_bytes(plistlib.dumps(entitlements))
        subprocess.run(["codesign", "--force", "--sign", "-", "--entitlements", str(path), "--generate-entitlement-der", str(bundle)], check=True)
    with ZipFile(output, "w", ZIP_DEFLATED) as archive:
        for path in sorted(payload.rglob("*")):
            if path.is_file(): archive.write(path, path.relative_to(Path(tmp)))
print(output)
