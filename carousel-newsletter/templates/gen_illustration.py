#!/usr/bin/env python3
"""
Generate an EVC-branded illustration for a carousel slide via Gemini 2.5 Flash
Image ("Nano Banana") on Vertex AI, using the EVC banner as a style reference.

Usage:
    python3 gen_illustration.py <scene> <output.png> [--aspect 4:5] [--force]

<scene>      — per-slide scene description (what goes in the frame).
<output>     — where to save the PNG.
--aspect     — 1:1, 4:5 (default; IG portrait), 16:9, or 9:16.
--force      — regenerate even if <output> already exists.

Auth:        gcloud Application Default Credentials. Run once:
                gcloud auth application-default login

Requires:    google-genai SDK, sips (macOS; for banner downscale).

Design:
- Attaches the EVC banner as a visual style reference. Nano Banana keys on
  image refs more tightly than on text descriptions alone.
- The master brand prompt locks palette, character design, and composition
  rules. Every slide starts from the same brand prompt; only the SCENE differs.
- Full 4.9 MB banner is silently dropped by the API, so we downscale to 1024px
  before attaching.
"""
from __future__ import annotations

import argparse
import base64
import os
import pathlib
import subprocess
import sys

TEMPLATES_DIR = pathlib.Path(__file__).resolve().parent
BANNER_SRC = pathlib.Path.home() / "Pictures" / "evc_banner2.png"
BANNER_CACHED = pathlib.Path("/tmp/evc-banner-1024.png")
PROJECT = os.environ.get("GOOGLE_CLOUD_PROJECT", "gen-lang-client-0527845499")
LOCATION = os.environ.get("GOOGLE_CLOUD_LOCATION", "us-central1")
MODEL = os.environ.get("EVC_IMAGE_MODEL", "gemini-2.5-flash-image")

BRAND_PROMPT = """\
Using the attached reference image as the EXACT style template, create a NEW \
illustration. Match the reference's: warm cream background with faint grid, \
friendly white astronaut-robot characters with blue visors, chunky isometric \
lego bricks in yellow/blue/green, bold navy outlines, cool gray gears with \
teeth, wooden rail tracks, painterly hand-drawn quality (not razor-sharp \
vector).

STRICT PALETTE — use only these, nothing else:
  Navy #14386B · Forest green #2D8A3E · Mustard yellow #F2B705
  Royal blue #2E6CC4 · Cool gray #8894A3 · Wood brown #8B5A2B
  White #FFFFFF · Cream #F5ECD7
No red, orange, purple, pink, teal, or other colors.

ROBOT CHARACTERS: rounded white padded-coverall bodies, spherical white \
helmets, royal-blue rounded-rectangle visor with 2 small light-blue eye dots \
showing through, small navy tie on the chest, white-gloved hands often \
holding a silver wrench. Slightly chibi/squat proportions.

LEGO BRICKS: chunky isometric 3D showing top face with studs + front + one \
side. Darker shade on one side for depth. Sizes 2x2, 2x4, 1x2 only. Only \
yellow, blue, or green from the palette. Bold navy outlines.

GEARS: cool gray with clearly visible individual teeth (not smooth). Visible \
center hub with a darker bolt. Multiple sizes.

RAILS: warm-brown wooden ties horizontal, two parallel cool-gray metal rails \
on top. Straight or gently curving. Sometimes elevated on brick or pillar \
supports as a mini bridge.

MINECART: gray metal, two visible wheels, slight isometric perspective. \
Bolts/rivets on sides. Often filled to the brim with yellow/blue/green lego \
bricks sticking out.

COMPOSITION:
- NO text, letters, words, numbers, captions, or logos anywhere.
- Cream background fills every edge.
- Small dark-navy triangle watermark bottom-right (~2% of canvas).
- Soft hand-drawn outline quality, not razor-sharp vector."""


def ensure_banner() -> bytes:
    if not BANNER_SRC.exists():
        sys.exit(f"banner not found at {BANNER_SRC}")
    if not BANNER_CACHED.exists() or BANNER_CACHED.stat().st_mtime < BANNER_SRC.stat().st_mtime:
        subprocess.run(
            ["sips", "-Z", "1024", str(BANNER_SRC), "--out", str(BANNER_CACHED)],
            capture_output=True, check=True,
        )
    return BANNER_CACHED.read_bytes()


def generate(scene: str, output: pathlib.Path, aspect: str = "4:5") -> None:
    # Late import so --help works without deps
    os.environ["GOOGLE_GENAI_USE_VERTEXAI"] = "true"
    os.environ.setdefault("GOOGLE_CLOUD_PROJECT", PROJECT)
    os.environ.setdefault("GOOGLE_CLOUD_LOCATION", LOCATION)

    from google import genai
    from google.genai import types

    banner = ensure_banner()
    client = genai.Client()

    prompt = f"{BRAND_PROMPT}\n\nSCENE (aspect {aspect}):\n{scene}"

    resp = client.models.generate_content(
        model=MODEL,
        contents=[
            types.Content(role="user", parts=[
                types.Part.from_text(text=prompt),
                types.Part.from_bytes(data=banner, mime_type="image/png"),
            ])
        ],
        config=types.GenerateContentConfig(response_modalities=["IMAGE"]),
    )

    for cand in resp.candidates or []:
        for part in (cand.content.parts if cand.content else []):
            if part.inline_data and part.inline_data.data:
                output.parent.mkdir(parents=True, exist_ok=True)
                output.write_bytes(part.inline_data.data)
                print(f"{output} ({output.stat().st_size} bytes)")
                return
    sys.exit(f"no image in response: {resp!r}")


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("scene", help="Scene description for the slide")
    ap.add_argument("output", type=pathlib.Path, help="Where to save the PNG")
    ap.add_argument("--aspect", default="4:5", choices=["1:1", "4:3", "3:4", "4:5", "16:9", "9:16"])
    ap.add_argument("--force", action="store_true", help="Regenerate even if output exists")
    args = ap.parse_args()

    if args.output.exists() and not args.force:
        print(f"{args.output} (cached, use --force to regenerate)")
        return

    generate(args.scene, args.output, args.aspect)


if __name__ == "__main__":
    main()
