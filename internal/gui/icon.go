package gui

import (
	"bytes"
	_ "embed"
	"image/color"
	"image/png"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/ui"
)

// 图标由 scripts/make-icon.py 生成；这里只打包运行时用得着的两档尺寸。
// 窗口图标用 256px（任务栏/Alt+Tab 都够清晰），标题栏品牌块用 48px
// （24 逻辑像素 × 2 倍屏）。
//
//go:embed icon-256.png
var windowIconPNG []byte

//go:embed icon-48.png
var brandIconPNG []byte

// windowIcon 把打包的图标解成 Viva 的窗口图标：Size×Size 的直通（非预乘）
// RGBA，top row first。PNG 每像素是直通 RGBA，所以用 NRGBA 模型取通道。
// 只在启动时算一次，不进绘制路径。
func windowIcon() app.WindowIcon {
	picture, err := png.Decode(bytes.NewReader(windowIconPNG))
	if err != nil {
		return app.WindowIcon{}
	}
	bounds := picture.Bounds()
	size := bounds.Dx()
	if bounds.Dy() < size {
		size = bounds.Dy()
	}
	pixels := make([]byte, size*size*4)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			value := color.NRGBAModel.Convert(picture.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.NRGBA)
			offset := (y*size + x) * 4
			pixels[offset+0] = value.R
			pixels[offset+1] = value.G
			pixels[offset+2] = value.B
			pixels[offset+3] = value.A
		}
	}
	return app.WindowIcon{Size: size, RGBA: pixels}
}

// brandBitmap 是标题栏品牌块用的位图。Viva 的位图是预乘 BGRA（与结果图预览
// 同一约定），同样只在启动时解一次。
func brandBitmap() ui.Bitmap {
	picture, err := png.Decode(bytes.NewReader(brandIconPNG))
	if err != nil {
		return ui.Bitmap{}
	}
	bounds := picture.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	pixels := make([]byte, width*height*4)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			value := color.NRGBAModel.Convert(picture.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.NRGBA)
			alpha := uint32(value.A)
			offset := (y*width + x) * 4
			pixels[offset+0] = uint8(uint32(value.B) * alpha / 255)
			pixels[offset+1] = uint8(uint32(value.G) * alpha / 255)
			pixels[offset+2] = uint8(uint32(value.R) * alpha / 255)
			pixels[offset+3] = value.A
		}
	}
	// ID 与结果图预览（preview.go 用 1）分开：渲染器按 ID 缓存纹理，两个
	// 位图同时在屏幕上时不能共用一个 ID。
	return ui.Bitmap{Pix: pixels, Width: width, Height: height, Stride: width * 4, ID: 2, Version: 1}
}

// paintBrand 把品牌图标画进标题栏占位块；矩形来自本帧布局（与结果图预览
// 同一套做法：flex 只占位，位图在 view.Draw 之后画）。
func (a *App) paintBrand(canvas *ui.Canvas) {
	if a.brand.Width == 0 || a.brandBox.Rect().Width < 1 {
		return
	}
	canvas.DrawBitmap(a.brand, a.brandBox.Rect())
}
