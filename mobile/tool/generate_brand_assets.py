"""Regenerate native Rub el Hizb PNGs from the geometry in assets/brand/logo.svg.

Run with Python and Pillow: python tool/generate_brand_assets.py
"""

import json
from pathlib import Path

from PIL import Image, ImageDraw


ROOT = Path(__file__).resolve().parents[1]
EMERALD = (27, 126, 60)
LIGHT_EMERALD = (76, 179, 104)
WHITE = (255, 255, 255)


def draw_mark(size, color, background=None, scale=0.86):
    """Rasterize the approved two squares and central circle with antialiasing."""
    factor = 4
    canvas = Image.new(
        "RGBA", (size * factor, size * factor),
        (*background, 255) if background else (0, 0, 0, 0),
    )
    draw = ImageDraw.Draw(canvas)
    unit = size * factor * scale / 100
    center = size * factor / 2

    def point(x, y):
        return (center + (x - 50) * unit, center + (y - 50) * unit)

    width = round(7 * unit)
    square = [point(24, 24), point(76, 24), point(76, 76), point(24, 76)]
    diamond = [point(50, 13.23), point(86.77, 50), point(50, 86.77), point(13.23, 50)]
    for vertices in (square, diamond):
        draw.line(vertices + vertices[:1], fill=(*color, 255), width=width, joint="curve")
    draw.ellipse([point(40, 40), point(60, 60)], outline=(*color, 255), width=width)
    return canvas.resize((size, size), Image.Resampling.LANCZOS)


def main():
    ios = ROOT / "ios/Runner/Assets.xcassets"
    icon_dir = ios / "AppIcon.appiconset"
    icon_set = json.loads((icon_dir / "Contents.json").read_text(encoding="utf-8"))
    for entry in icon_set["images"]:
        points = float(entry["size"].split("x")[0])
        pixels = round(points * int(entry["scale"][0]))
        draw_mark(pixels, WHITE, EMERALD).convert("RGB").save(icon_dir / entry["filename"])

    android = ROOT / "android/app/src/main/res"
    for density, pixels in (("mdpi", 48), ("hdpi", 72), ("xhdpi", 96),
                            ("xxhdpi", 144), ("xxxhdpi", 192)):
        draw_mark(pixels, WHITE, EMERALD).save(android / f"mipmap-{density}/ic_launcher.png")

    launch_dir = ios / "LaunchImage.imageset"
    for scale, suffix in ((1, ""), (2, "@2x"), (3, "@3x")):
        pixels = 128 * scale
        draw_mark(pixels, EMERALD, scale=1).save(launch_dir / f"LaunchImage{suffix}.png")
        draw_mark(pixels, LIGHT_EMERALD, scale=1).save(
            launch_dir / f"LaunchImage{suffix}-dark.png"
        )


if __name__ == "__main__":
    main()
