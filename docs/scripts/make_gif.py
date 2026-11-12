"""Monta o GIF a partir dos quadros e durações de frames/frames.txt."""

import sys
from pathlib import Path

from PIL import Image

frames_dir, out = Path(sys.argv[1]), Path(sys.argv[2])
width = int(sys.argv[3]) if len(sys.argv) > 3 else 0

lines = (frames_dir / "frames.txt").read_text().splitlines()
names, durations = [], []
for line in lines:
    if line.startswith("file "):
        names.append(line.split("'")[1])
    elif line.startswith("duration "):
        durations.append(round(float(line.split()[1]) * 1000))
names = names[: len(durations)]  # o último "file" repetido é só do formato do ffmpeg

images = []
for name in names:
    img = Image.open(frames_dir / name).convert("RGB")
    if width and img.width != width:
        img = img.resize((width, round(img.height * width / img.width)), Image.LANCZOS)
    # Paleta própria por quadro, sem pontilhado: a interface tem cores chapadas.
    images.append(img.quantize(colors=128, method=Image.Quantize.MEDIANCUT, dither=Image.Dither.NONE))

images[0].save(out, save_all=True, append_images=images[1:], duration=durations, loop=0, optimize=True, disposal=1)
print(f"{out}: {len(images)} quadros, {sum(durations) / 1000:.1f} s, {out.stat().st_size / 1e6:.2f} MB")
