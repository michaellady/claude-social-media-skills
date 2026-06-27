#!/usr/bin/env python3
"""
Generate an EVC-branded illustration for a carousel slide via OpenAI's
gpt-image-1, using the EVC banner as a style reference.

Drop-in alternative to gen_illustration.py (Gemini "Nano Banana"). Same CLI
interface and the SAME locked BRAND_PROMPT (imported from gen_illustration) so
the two engines can be A/B compared on identical inputs.

Usage:
    python3 gen_illustration_openai.py <scene> <output.png> [--aspect 9:16] \
        [--quality high] [--force]

<scene>      — per-slide scene description (what goes in the frame).
<output>     — where to save the PNG.
--aspect     — 1:1, 4:3, 3:4, 4:5, 16:9, or 9:16. Mapped to the nearest size
               gpt-image-1 supports; the two-zone HTML layout crops to fill the
               slide's illustration zone, so exact aspect is not critical.
--quality    — low | medium | high | auto (default high).
--force      — regenerate even if <output> already exists.

Auth:        OPENAI_API_KEY env var (platform API key with image access — a
             ChatGPT/codex login token will NOT work for the Images API).

Requires:    openai SDK (pip3 install --user --break-system-packages openai),
             sips (macOS; reused from gen_illustration for the banner downscale).

Design:
- Uses images.edit (NOT images.generate) so the EVC banner can be passed as a
  visual reference — gpt-image-1 keys on the reference image the way Nano Banana
  keys on its style ref. images.generate has no reference-image parameter.
- gpt-image-1 only emits 1024x1024, 1536x1024, or 1024x1536. We pick the nearest
  by orientation; the HTML render crops to the zone regardless.
- gpt-image-1 always returns base64 (no URL response mode).
"""
from __future__ import annotations

import argparse
import base64
import os
import pathlib
import sys
import time

# Single source of truth for the brand prompt + banner handling. Importing this
# module does NOT pull in google-genai (that import is lazy, inside generate()).
sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
from gen_illustration import BRAND_PROMPT, BANNER_CACHED, ensure_banner  # noqa: E402

MODEL = os.environ.get("EVC_OPENAI_IMAGE_MODEL", "gpt-image-1")

# gpt-image-1 supported sizes, chosen by orientation of the requested aspect.
ASPECT_TO_SIZE = {
    "1:1": "1024x1024",
    "4:3": "1536x1024",
    "16:9": "1536x1024",
    "3:4": "1024x1536",
    "4:5": "1024x1536",
    "9:16": "1024x1536",
}


def generate(scene: str, output: pathlib.Path, aspect: str = "9:16",
             quality: str = "high", retry_429: int = 3, backoff_s: int = 30) -> None:
    # Late import so --help works without the SDK installed.
    try:
        from openai import OpenAI
    except ModuleNotFoundError:
        sys.exit("openai SDK not installed — run: "
                 "pip3 install --user --break-system-packages openai")

    if not os.environ.get("OPENAI_API_KEY"):
        sys.exit("OPENAI_API_KEY not set — a platform API key with image access "
                 "is required (a codex/ChatGPT login token will not work).")

    ensure_banner()  # refresh /tmp/evc-banner-1024.png from ~/Pictures/evc_banner2.png
    size = ASPECT_TO_SIZE[aspect]
    prompt = f"{BRAND_PROMPT}\n\nSCENE (aspect {aspect}):\n{scene}"
    client = OpenAI()

    attempt = 0
    while True:
        attempt += 1
        try:
            with open(BANNER_CACHED, "rb") as banner_fh:
                resp = client.images.edit(
                    model=MODEL,
                    image=[banner_fh],
                    prompt=prompt,
                    size=size,
                    quality=quality,
                )
            break
        except Exception as e:  # noqa: BLE001 — surface API errors with context
            is_429 = "429" in str(e) or "rate limit" in str(e).lower()
            if is_429 and attempt <= retry_429:
                wait = backoff_s * attempt
                print(f"  429 rate limit (attempt {attempt}/{retry_429}); sleeping {wait}s",
                      file=sys.stderr)
                time.sleep(wait)
                continue
            # gpt-image-1 requires org verification; make that failure legible.
            if "organization" in str(e).lower() and "verif" in str(e).lower():
                sys.exit("gpt-image-1 requires a VERIFIED OpenAI organization. "
                         "Verify at platform.openai.com/settings/organization/general, "
                         f"then retry.\n\nUnderlying error: {e}")
            raise

    b64 = resp.data[0].b64_json
    if not b64:
        sys.exit(f"no image in response: {resp!r}")
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_bytes(base64.b64decode(b64))
    print(f"{output} ({output.stat().st_size} bytes, {MODEL} {quality} {size})")


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("scene", help="Scene description for the slide")
    ap.add_argument("output", type=pathlib.Path, help="Where to save the PNG")
    ap.add_argument("--aspect", default="9:16",
                    choices=["1:1", "4:3", "3:4", "4:5", "16:9", "9:16"])
    ap.add_argument("--quality", default="high",
                    choices=["low", "medium", "high", "auto"])
    ap.add_argument("--force", action="store_true",
                    help="Regenerate even if output exists")
    ap.add_argument("--retry-on-429", type=int, default=3, dest="retry_429",
                    help="Retries on 429 rate limit before giving up (default: 3)")
    ap.add_argument("--backoff-s", type=int, default=30, dest="backoff_s",
                    help="Base backoff seconds; multiplied by attempt number (default: 30)")
    args = ap.parse_args()

    if args.output.exists() and not args.force:
        print(f"{args.output} (cached, use --force to regenerate)")
        return

    generate(args.scene, args.output, args.aspect, quality=args.quality,
             retry_429=args.retry_429, backoff_s=args.backoff_s)


if __name__ == "__main__":
    main()
