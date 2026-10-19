"""Corporate-style icon: navy tile, white isometric package, teal gear badge."""
import math, sys
from PIL import Image, ImageDraw

S = 1024
img = Image.new("RGBA", (S, S), (0, 0, 0, 0))

# tile with subtle vertical gradient
tile = Image.new("RGBA", (S, S))
td = ImageDraw.Draw(tile)
top, bot = (30, 64, 120), (16, 38, 78)
for y in range(S):
    t = y / S
    td.line([(0, y), (S, y)], fill=tuple(int(top[i] * (1 - t) + bot[i] * t) for i in range(3)) + (255,))
mask = Image.new("L", (S, S), 0)
ImageDraw.Draw(mask).rounded_rectangle((40, 40, S - 40, S - 40), radius=190, fill=255)
img.paste(tile, (0, 0), mask)
d = ImageDraw.Draw(img)

# isometric package (box), white line art
W = (255, 255, 255, 255)
LW = 46
cx, cy = 470, 470
a, h = 250, 270           # half-width of top diamond, height of sides
dy = int(a * 0.55)
T = (cx, cy - dy - h // 2)            # top
L = (cx - a, cy - h // 2)             # left
R = (cx + a, cy - h // 2)             # right
C = (cx, cy + dy - h // 2)            # center (front corner of top)
Lb = (L[0], L[1] + h); Rb = (R[0], R[1] + h); Cb = (C[0], C[1] + h)

# faces (slight tints so it reads as a solid object)
d.polygon([L, C, Cb, Lb], fill=(58, 92, 146, 255))
d.polygon([C, R, Rb, Cb], fill=(44, 76, 128, 255))
d.polygon([T, R, C, L], fill=(78, 112, 166, 255))
for p, q in [(T, R), (R, C), (C, L), (L, T), (L, Lb), (Lb, Cb), (Cb, Rb), (Rb, R), (C, Cb)]:
    d.line([p, q], fill=W, width=LW)
for p in (T, R, C, L, Lb, Cb, Rb):
    d.ellipse((p[0] - LW // 2, p[1] - LW // 2, p[0] + LW // 2, p[1] + LW // 2), fill=W)
# tape strip across the top
m1 = ((T[0] + L[0]) / 2, (T[1] + L[1]) / 2); m2 = ((R[0] + C[0]) / 2, (R[1] + C[1]) / 2)
d.line([m1, m2], fill=W, width=LW - 14)

# gear badge bottom-right
gx, gy, ro, ri, teeth = 735, 735, 205, 165, 8
d.ellipse((gx - ro - 34, gy - ro - 34, gx + ro + 34, gy + ro + 34), fill=(16, 38, 78, 255))  # cut-out ring
pts = []
n = teeth * 4
for i in range(n):
    ang = 2 * math.pi * i / n + math.pi / n
    r = ro if (i % 4) in (0, 1) else ri
    pts.append((gx + r * math.cos(ang), gy + r * math.sin(ang)))
d.polygon(pts, fill=(38, 196, 170, 255))
d.ellipse((gx - 140, gy - 140, gx + 140, gy + 140), fill=(38, 196, 170, 255))
d.ellipse((gx - 70, gy - 70, gx + 70, gy + 70), fill=(16, 38, 78, 255))

big = img.resize((256, 256), Image.LANCZOS)
big.save(sys.argv[1], sizes=[(16, 16), (24, 24), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256)])
prev = Image.new("RGBA", (720, 296), (240, 242, 246, 255))
prev.alpha_composite(big, (10, 20))
dark = Image.new("RGBA", (256, 256), (32, 32, 36, 255)); dark.alpha_composite(big); prev.alpha_composite(dark, (276, 20))
y = 20
for s in (16, 32, 48):
    sm = img.resize((s, s), Image.LANCZOS).resize((s * 2, s * 2), Image.NEAREST)
    prev.alpha_composite(sm, (560, y)); y += s * 2 + 10
prev.save(sys.argv[2])
