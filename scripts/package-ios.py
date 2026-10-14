#!/usr/bin/env python3
"""Create a Mac handoff containing source only, never keys or receiver state."""
from pathlib import Path
from zipfile import ZipFile, ZIP_DEFLATED

root = Path(__file__).resolve().parents[1]
destination = root / "bin" / "NearDrop-iPhone.zip"
destination.parent.mkdir(exist_ok=True)
with ZipFile(destination, "w", ZIP_DEFLATED) as archive:
    for folder in ["ios", "scripts", "docs"]:
        for path in sorted((root / folder).rglob("*")):
            if not path.is_file() or any(part in {"xcuserdata", "DerivedData", "__pycache__"} for part in path.parts):
                continue
            if path.name == "Local.xcconfig" or path.suffix in {".xcuserstate", ".zip"}:
                continue
            archive.write(path, Path("NearDrop") / path.relative_to(root))
    archive.write(root / "README.md", "NearDrop/README.md")
print(destination)
