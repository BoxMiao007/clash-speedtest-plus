#!/usr/bin/env python3
"""生成订阅测速的应用图标（SVG 源 + 多尺寸 PNG + Windows .ico）。

设计：深色圆角方块 + 蓝色表盘弧 + 白色闪电。表盘弧表达「测速」，
闪电表达「高速」，两者在 16px 任务栏尺寸下仍是两个干净的大形状。

用法：
  python scripts/make-icon.py                 # 生成 assets/icon/ 全套
  python scripts/make-icon.py --preview DIR   # 额外输出对比图到 DIR

只依赖 Pillow 与 numpy（仓库其它脚本不需要它们，本脚本只在改图标时跑）。
"""

import argparse
import os
import sys

import numpy as np
from PIL import Image, ImageDraw, ImageFont

SS = 4  # 超采样倍数：先画 4 倍，再 LANCZOS 缩到目标尺寸

TILE_TOP = (0x17, 0x1F, 0x2E)
TILE_BOTTOM = (0x08, 0x0B, 0x11)
GLOW = (0x2E, 0x6B, 0xFF)
# 表盘弧用三段亮蓝：最暗的一段也不能沉进深色底，否则 16px 下半个环会消失。
ACCENT_TOP = (0x8B, 0xDC, 0xFF)   # sky-200
ACCENT_MID = (0x4C, 0x8D, 0xFF)   # 与深色主题的 palette.Accent 同色
ACCENT_BOTTOM = (0x25, 0x63, 0xEB)  # blue-600
BOLT = (0xF8, 0xFA, 0xFC)


def hex_svg(color):
    return "#%02X%02X%02X" % color


def tile_background(size):
    """深色竖向渐变 + 中心偏上的蓝色辉光，给图标一点纵深。"""
    y = np.linspace(0.0, 1.0, size, dtype=np.float32)[:, None]
    top = np.array(TILE_TOP, dtype=np.float32)
    bottom = np.array(TILE_BOTTOM, dtype=np.float32)
    rows = top[None, :] * (1.0 - y) + bottom[None, :] * y  # (size,3)
    img = np.repeat(rows[:, None, :], size, axis=1)

    # 径向辉光：中心 (0.5, 0.42)，半径 0.62，强度从 0.30 衰减到 0。
    yy, xx = np.mgrid[0:size, 0:size].astype(np.float32) / float(size)
    dist = np.sqrt((xx - 0.5) ** 2 + (yy - 0.42) ** 2)
    glow = np.clip(1.0 - dist / 0.62, 0.0, 1.0) ** 2 * 0.30
    glow = glow[:, :, None]
    img = img * (1.0 - glow) + np.array(GLOW, dtype=np.float32)[None, None, :] * glow
    return Image.fromarray(np.clip(img, 0, 255).astype(np.uint8), "RGB").convert("RGBA")


def rounded_mask(size, radius):
    mask = Image.new("L", (size, size), 0)
    ImageDraw.Draw(mask).rounded_rectangle([0, 0, size - 1, size - 1], radius=radius, fill=255)
    return mask


def mix(a, b, t):
    return tuple(int(round(a[i] + (b[i] - a[i]) * t)) for i in range(3))


def arc_polyline(cx, cy, radius, start_deg, end_deg, steps):
    """PIL 的角度：0=右侧，90=下方（y 向下），顺时针。"""
    pts = []
    for i in range(steps + 1):
        t = i / steps
        deg = start_deg + (end_deg - start_deg) * t
        rad = np.deg2rad(deg)
        pts.append((cx + radius * np.cos(rad), cy + radius * np.sin(rad)))
    return pts


def ring_colors(t):
    if t < 0.5:
        return mix(ACCENT_TOP, ACCENT_MID, t / 0.5)
    return mix(ACCENT_MID, ACCENT_BOTTOM, (t - 0.5) / 0.5)


def draw_ring(img, cx, cy, radius, width, start_deg, end_deg):
    """画表盘弧：先用 mask 描出平滑的弧 + 圆头，再沿 45° 方向铺渐变。

    不要用逐段 draw.line 拼弧：线段接缝会在边缘留下扇贝状锯齿（256px 可见）。
    """
    s = img.width
    w = max(1, int(round(width)))
    mask = Image.new("L", (s, s), 0)
    pen = ImageDraw.Draw(mask)
    # Pillow 的 arc(width) 从 box 边界向内描，所以把 box 放大半个线宽：这样
    # radius 就是线带中心线，圆头也画在同一条中心线上，接头不会鼓出一个小球。
    outer = radius + w / 2.0
    box = [cx - outer, cy - outer, cx + outer, cy + outer]
    pen.arc(box, start_deg, end_deg, fill=255, width=w)
    r = w / 2.0
    for deg in (start_deg, end_deg):
        rad = np.deg2rad(deg)
        px, py = cx + radius * np.cos(rad), cy + radius * np.sin(rad)
        pen.ellipse([px - r, py - r, px + r, py + r], fill=255)
    del pen

    # 渐变的参数 t 沿「右上 → 左下」方向变化，与弧的走向基本一致。
    yy, xx = np.mgrid[0:s, 0:s].astype(np.float32) / float(s)
    t = np.clip(0.5 + (xx - 0.5) * 0.95 - (yy - 0.5) * 0.75, 0.0, 1.0)
    flat = t.ravel()
    palette = np.array([ring_colors(float(v)) for v in np.linspace(0, 1, 256)], dtype=np.uint8)
    index = np.clip((flat * 255).astype(np.int32), 0, 255)
    gradient = palette[index].reshape(s, s, 3)
    layer = Image.fromarray(gradient, "RGB").convert("RGBA")
    img.paste(layer, (0, 0), mask)


def bolt_polygon(cx, cy, height, width, skew=0.10):
    """经典闪电：上臂向右下，下臂向左下，中间一个横向台阶。"""
    h = height / 2.0
    w = width / 2.0
    points = [
        (cx + w * 0.62 + skew * w, cy - h),      # 右上
        (cx - w * 0.96, cy + h * 0.06),          # 中左
        (cx - w * 0.10, cy + h * 0.06),          # 台阶右
        (cx - w * 0.62, cy + h),                 # 左下
        (cx + w * 0.96, cy - h * 0.06),          # 中右
        (cx + w * 0.10, cy - h * 0.06),          # 台阶左
    ]
    return points


def render_dial(size):
    """方案 A：表盘弧 + 闪电。"""
    s = size * SS
    img = tile_background(s)
    draw = ImageDraw.Draw(img)
    cx = cy = s / 2.0
    draw_ring(img, cx, cy, radius=s * 0.293, width=s * 0.060, start_deg=116, end_deg=424)
    draw.polygon(bolt_polygon(cx, cy, height=s * 0.360, width=s * 0.196), fill=BOLT)
    del draw
    mask = rounded_mask(s, int(s * 0.218))
    out = Image.new("RGBA", (s, s), (0, 0, 0, 0))
    out.paste(img, (0, 0), mask)
    return out.resize((size, size), Image.LANCZOS)


def render_bolt(size):
    """方案 B：纯闪电 + 细弧点缀。"""
    s = size * SS
    img = tile_background(s)
    draw = ImageDraw.Draw(img)
    cx = cy = s / 2.0
    draw_ring(img, cx, cy, radius=s * 0.330, width=s * 0.030, start_deg=140, end_deg=400)
    draw.polygon(bolt_polygon(cx, cy, height=s * 0.470, width=s * 0.256), fill=BOLT)
    del draw
    mask = rounded_mask(s, int(s * 0.218))
    out = Image.new("RGBA", (s, s), (0, 0, 0, 0))
    out.paste(img, (0, 0), mask)
    return out.resize((size, size), Image.LANCZOS)


def render_dial2(size):
    """方案 D：细表盘弧 + 大闪电，两者不打架。"""
    s = size * SS
    img = tile_background(s)
    draw = ImageDraw.Draw(img)
    cx = cy = s / 2.0
    draw_ring(img, cx, cy, radius=s * 0.318, width=s * 0.058, start_deg=122, end_deg=418)
    draw.polygon(bolt_polygon(cx, cy, height=s * 0.385, width=s * 0.208), fill=BOLT)
    del draw
    mask = rounded_mask(s, int(s * 0.218))
    out = Image.new("RGBA", (s, s), (0, 0, 0, 0))
    out.paste(img, (0, 0), mask)
    return out.resize((size, size), Image.LANCZOS)


def render_bars(size):
    """方案 C：三根上升的速度条 + 端点。"""
    s = size * SS
    img = tile_background(s)
    draw = ImageDraw.Draw(img)
    cx = cy = s / 2.0
    bar_w = s * 0.108
    gap = s * 0.058
    baseline = cy + s * 0.215
    heights = (s * 0.190, s * 0.310, s * 0.450)
    colors = (ACCENT_BOTTOM, ACCENT_MID, ACCENT_TOP)
    total = bar_w * 3 + gap * 2
    left = cx - total / 2.0
    for i, (h, color) in enumerate(zip(heights, colors)):
        x0 = left + i * (bar_w + gap)
        draw.rounded_rectangle([x0, baseline - h, x0 + bar_w, baseline], radius=bar_w / 2.0, fill=color)
    del draw
    mask = rounded_mask(s, int(s * 0.218))
    out = Image.new("RGBA", (s, s), (0, 0, 0, 0))
    out.paste(img, (0, 0), mask)
    return out.resize((size, size), Image.LANCZOS)


VARIANTS = {"dial": render_dial, "dial2": render_dial2, "bolt": render_bolt, "bars": render_bars}


def contact_sheet(path):
    """把三个方案在深浅底色上按真实尺寸排一张对比图，便于挑方案。"""
    sizes = [16, 24, 32, 48, 64, 128]
    pad = 24
    row_h = 128 + pad * 2
    label_w = 110
    col_w = sum(s + pad * 2 for s in sizes)
    width = label_w + col_w
    height = row_h * len(VARIANTS) * 2
    sheet = Image.new("RGB", (width, height), (255, 255, 255))
    draw = ImageDraw.Draw(sheet)
    for band, bg in enumerate([(255, 255, 255), (11, 14, 20)]):
        draw.rectangle([0, band * row_h * len(VARIANTS), width, (band + 1) * row_h * len(VARIANTS)], fill=bg)
    y = 0
    fg = (20, 20, 20)
    for name, render in VARIANTS.items():
        for band in range(2):
            fg = (20, 20, 20) if band == 0 else (230, 235, 245)
            draw.text((12, y + row_h / 2 - 6), name if band == 0 else "", fill=fg)
            x = label_w
            for size in sizes:
                icon = render(size)
                sheet.paste(icon, (x + pad, y + pad + (128 - size) // 2), icon)
                draw.text((x + pad, y + row_h - 24), f"{size}", fill=fg)
                x += size + pad * 2
            y += row_h
    sheet.save(path)


def zoom_sheet(path, factor=10):
    """把 16/20/24/32 放大排出来，检查小尺寸下的可读性。"""
    sizes = [16, 20, 24, 32]
    pad = 16
    cell = max(sizes) * factor
    row_h = cell + pad * 2 + 18
    label_w = 90
    width = label_w + (cell + pad) * len(sizes)
    height = row_h * len(VARIANTS) * 2
    sheet = Image.new("RGB", (width, height), (255, 255, 255))
    draw = ImageDraw.Draw(sheet)
    y = 0
    for band in range(2):
        fg = (20, 20, 20) if band == 0 else (230, 235, 245)
        for name, render in VARIANTS.items():
            draw.rectangle([0, y, width, y + row_h], fill=(255, 255, 255) if band == 0 else (11, 14, 20))
            draw.text((10, y + row_h / 2), name if band == 0 else "", fill=fg)
            x = label_w
            for size in sizes:
                icon = render(size).resize((size * factor, size * factor), Image.NEAREST)
                sheet.paste(icon, (x + pad // 2, y + pad // 2 + (cell - size * factor) // 2), icon)
                draw.text((x + pad // 2, y + row_h - 16), f"{size}px", fill=fg)
                x += cell + pad
            y += row_h
    sheet.save(path)


def build_svg():
    """按同一组参数输出 SVG 源，方便以后改色/改比例后再导出 PNG。"""
    design = 1024.0
    cx = cy = design / 2.0
    radius = 0.318 * design
    stroke = 0.058 * design
    start, end = 122.0, 418.0
    a0, a1 = np.deg2rad(start), np.deg2rad(end)
    x0, y0 = cx + radius * np.cos(a0), cy + radius * np.sin(a0)
    x1, y1 = cx + radius * np.cos(a1), cy + radius * np.sin(a1)
    bolt = " ".join(f"{x:.1f},{y:.1f}" for x, y in bolt_polygon(cx, cy, design * 0.385, design * 0.208))
    return f'''<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1024 1024" width="1024" height="1024">
  <title>订阅测速 · Clash/Mihomo 节点测速</title>
  <!-- 表盘弧表达「测速」，闪电表达「高速」；两者在 16px 下仍是干净的大形状。 -->
  <defs>
    <linearGradient id="tile" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="{hex_svg(TILE_TOP)}"/>
      <stop offset="1" stop-color="{hex_svg(TILE_BOTTOM)}"/>
    </linearGradient>
    <radialGradient id="glow" cx="50%" cy="42%" r="62%">
      <stop offset="0" stop-color="{hex_svg(GLOW)}" stop-opacity="0.30"/>
      <stop offset="1" stop-color="{hex_svg(GLOW)}" stop-opacity="0"/>
    </radialGradient>
    <linearGradient id="ring" x1="70%" y1="8%" x2="30%" y2="92%">
      <stop offset="0" stop-color="{hex_svg(ACCENT_TOP)}"/>
      <stop offset="0.5" stop-color="{hex_svg(ACCENT_MID)}"/>
      <stop offset="1" stop-color="{hex_svg(ACCENT_BOTTOM)}"/>
    </linearGradient>
  </defs>
  <rect x="0" y="0" width="1024" height="1024" rx="{0.218 * design:.1f}" fill="url(#tile)"/>
  <rect x="0" y="0" width="1024" height="1024" rx="{0.218 * design:.1f}" fill="url(#glow)"/>
  <path d="M {x0:.1f} {y0:.1f} A {radius:.1f} {radius:.1f} 0 1 1 {x1:.1f} {y1:.1f}"
        fill="none" stroke="url(#ring)" stroke-width="{stroke:.1f}" stroke-linecap="round"/>
  <polygon points="{bolt}" fill="{hex_svg(BOLT)}"/>
</svg>
'''


def final_sheet(path, render):
    """定稿尺寸表：只放一个方案，深浅底各一行，含 256px 大图。"""
    sizes = [16, 24, 32, 48, 64, 128, 256]
    pad = 20
    row_h = 256 + pad * 2 + 22
    width = pad + sum(s + pad for s in sizes)
    height = row_h * 2
    sheet = Image.new("RGB", (width, height), (255, 255, 255))
    draw = ImageDraw.Draw(sheet)
    y = 0
    for band in range(2):
        bg = (255, 255, 255) if band == 0 else (11, 14, 20)
        fg = (20, 20, 20) if band == 0 else (230, 235, 245)
        draw.rectangle([0, y, width, y + row_h], fill=bg)
        x = pad
        for size in sizes:
            icon = render(size)
            sheet.paste(icon, (x, y + pad + (256 - size) // 2), icon)
            draw.text((x, y + row_h - 18), f"{size}", fill=fg)
            x += size + pad
        y += row_h
    sheet.save(path)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--preview", default="")
    parser.add_argument("--variant", default="dial", choices=sorted(VARIANTS))
    args = parser.parse_args()

    root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    out_dir = os.path.join(root, "assets", "icon")
    os.makedirs(out_dir, exist_ok=True)

    render = VARIANTS[args.variant]
    sizes = [16, 24, 32, 48, 64, 128, 256, 512, 1024]
    images = {}
    for size in sizes:
        icon = render(size)
        images[size] = icon
        icon.save(os.path.join(out_dir, f"app-icon-{size}.png"))
    images[256].save(os.path.join(out_dir, "app-icon.ico"),
                     sizes=[(16, 16), (24, 24), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256)])
    with open(os.path.join(out_dir, "app-icon.svg"), "w", encoding="utf-8") as handle:
        handle.write(build_svg())

    # 运行时要用的一份：embed 只能引用包内文件，所以留一份 256px 的副本在
    # internal/gui/ 下，由 icon.go 用 go:embed 打进二进制。
    runtime_256 = os.path.join(root, "internal", "gui", "icon-256.png")
    runtime_48 = os.path.join(root, "internal", "gui", "icon-48.png")
    images[256].save(runtime_256)
    images[48].save(runtime_48)
    print("wrote", out_dir, "and", runtime_256, runtime_48)

    if args.preview:
        os.makedirs(args.preview, exist_ok=True)
        sheet = os.path.join(args.preview, "contact-sheet.png")
        contact_sheet(sheet)
        zoom = os.path.join(args.preview, "zoom-sheet.png")
        zoom_sheet(zoom)
        final = os.path.join(args.preview, "final-sheet.png")
        final_sheet(final, render)
        print("preview", sheet, zoom, final)


if __name__ == "__main__":
    main()
