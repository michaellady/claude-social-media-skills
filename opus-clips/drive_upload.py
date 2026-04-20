#!/usr/bin/env python3
"""Upload a local video to the opus-clips-automation Drive folder via service account.

Used by the opus-clips skill to stage a livestream in Drive so Opus's in-page
Google Drive picker can consume it — bypassing the browser file chooser that
requires a real user gesture.

Usage:
  drive_upload.py /path/to/video.mp4
  drive_upload.py /path/to/video.mp4 --folder-id 12Hhu9... --sa-key ~/.config/opus-clips/sa-key.json

Emits JSON: {"id": "...", "name": "...", "webViewLink": "...", "mimeType": "..."}
"""
from __future__ import annotations

import argparse
import json
import pathlib
import sys
import time

DEFAULT_FOLDER = "12Hhu9UbyVV6-bf5uACL7_aJRJYuk3dC1"
DEFAULT_SA_KEY = pathlib.Path.home() / ".config" / "opus-clips" / "sa-key.json"
SCOPES = ["https://www.googleapis.com/auth/drive.file"]


def upload(video: pathlib.Path, folder_id: str, sa_key_path: pathlib.Path) -> dict:
    from google.oauth2 import service_account
    from googleapiclient.discovery import build
    from googleapiclient.http import MediaFileUpload

    creds = service_account.Credentials.from_service_account_file(
        str(sa_key_path),
        scopes=["https://www.googleapis.com/auth/drive"],
    )
    drive = build("drive", "v3", credentials=creds, cache_discovery=False)

    media = MediaFileUpload(str(video), mimetype="video/mp4", resumable=True, chunksize=8 * 1024 * 1024)
    body = {"name": video.name, "parents": [folder_id]}
    req = drive.files().create(body=body, media_body=media, fields="id,name,webViewLink,mimeType,size")
    resp = None
    t0 = time.time()
    while resp is None:
        status, resp = req.next_chunk()
        if status:
            pct = int(status.progress() * 100)
            elapsed = time.time() - t0
            print(f"  {pct:3d}% ({elapsed:.1f}s)", file=sys.stderr)
    print(f"  done in {time.time()-t0:.1f}s", file=sys.stderr)
    return resp


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("video", type=pathlib.Path, help="Absolute path to local video")
    p.add_argument("--folder-id", default=DEFAULT_FOLDER, help="Drive folder ID (default: opus-clips-automation)")
    p.add_argument("--sa-key", type=pathlib.Path, default=DEFAULT_SA_KEY, help="Service account JSON key path")
    args = p.parse_args(argv)

    if not args.video.exists():
        print(f"error: file not found: {args.video}", file=sys.stderr)
        return 2
    if not args.sa_key.exists():
        print(f"error: SA key not found: {args.sa_key}", file=sys.stderr)
        return 2

    result = upload(args.video, args.folder_id, args.sa_key)
    json.dump(result, sys.stdout, indent=2)
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
