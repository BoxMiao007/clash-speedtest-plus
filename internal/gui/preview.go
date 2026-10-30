package gui

import (
	"errors"
	"image"
	"image/png"
	"os"
	"strings"

	"github.com/vivaui/viva/components"
	"github.com/vivaui/viva/ui"
)

// 结果图预览的固定允许上限：超过后拒绝解码，避免一张超大图片把内存吃光。
const maxPreviewPixels = 60_000_000

// startImagePreview 打开结果图预览。解码在后台 goroutine 完成，窗口线程只
// 接收 pending 事件；绘制路径里不做文件 I/O。
func (s *state) startImagePreview(path string) {
	if strings.TrimSpace(path) == "" {
		s.status = "还没有结果图，先开测或手动导出"
		return
	}
	if _, err := os.Stat(path); err != nil {
		s.status = "结果图不存在：" + path
		return
	}
	s.preview = previewState{
		open:    true,
		loading: true,
		path:    path,
		version: s.preview.version + 1,
		text:    path + " · 正在解码…",
	}
	s.dialog = dialogPreview
	version := s.preview.version
	go func() {
		bitmap, err := loadPreviewBitmap(path)
		s.enqueue(runEvent{kind: eventPreview, path: path, bitmap: bitmap, version: version, err: err})
	}()
}

// applyPreview 把解码结果并入预览状态；连续点预览时只接受最后一次的结果。
func (s *state) applyPreview(event runEvent) {
	if !s.preview.open || event.version != s.preview.version {
		return
	}
	s.preview.loading = false
	if event.err != nil {
		s.preview.err = event.err.Error()
		s.preview.text = "预览失败：" + s.preview.err
		return
	}
	s.preview.bitmap = event.bitmap
	s.preview.err = ""
	s.preview.text = s.preview.path + " · " + itoa(event.bitmap.Width) + "×" + itoa(event.bitmap.Height)
}

// togglePreviewZoom 在「适应窗口」与「原始大小」之间切换。
func (s *state) togglePreviewZoom() {
	if s.preview.zoom == 0 {
		s.preview.zoom = 1
		return
	}
	s.preview.zoom = 0
}

// previewZoomLabel 返回缩放按钮上的文案（预计算，绘制里不拼串）。
func (s *state) previewZoomLabel() string {
	if s.preview.zoom == 0 {
		return "原始大小"
	}
	return "适应窗口"
}

// loadPreviewBitmap 把 PNG 解码成 Canvas 需要的预乘 BGRA 位图。结果图是
// 我们自己写出的 PNG，但仍按通用解码路径处理，避免格式假设。
func loadPreviewBitmap(path string) (ui.Bitmap, error) {
	file, err := os.Open(path)
	if err != nil {
		return ui.Bitmap{}, err
	}
	defer file.Close()
	picture, err := png.Decode(file)
	if err != nil {
		return ui.Bitmap{}, err
	}
	return bitmapFromImage(picture)
}

func bitmapFromImage(picture image.Image) (ui.Bitmap, error) {
	bounds := picture.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return ui.Bitmap{}, errors.New("结果图尺寸无效")
	}
	if width*height > maxPreviewPixels {
		return ui.Bitmap{}, errors.New("结果图过大，无法预览")
	}
	stride := width * 4
	pix := make([]byte, stride*height)
	switch source := picture.(type) {
	case *image.NRGBA:
		// NRGBA 是非预乘的；Canvas 要预乘 BGRA，逐像素乘 alpha 并换通道序。
		for y := 0; y < height; y++ {
			src := source.Pix[y*source.Stride : y*source.Stride+width*4]
			dst := pix[y*stride : y*stride+width*4]
			for x := 0; x < width; x++ {
				alpha := uint32(src[x*4+3])
				dst[x*4+0] = uint8(uint32(src[x*4+2]) * alpha / 255)
				dst[x*4+1] = uint8(uint32(src[x*4+1]) * alpha / 255)
				dst[x*4+2] = uint8(uint32(src[x*4+0]) * alpha / 255)
				dst[x*4+3] = uint8(alpha)
			}
		}
	case *image.RGBA:
		// RGBA 已经是预乘的，只需要把通道序从 RGBA 换成 BGRA。
		for y := 0; y < height; y++ {
			src := source.Pix[y*source.Stride : y*source.Stride+width*4]
			dst := pix[y*stride : y*stride+width*4]
			for x := 0; x < width; x++ {
				dst[x*4+0] = src[x*4+2]
				dst[x*4+1] = src[x*4+1]
				dst[x*4+2] = src[x*4+0]
				dst[x*4+3] = src[x*4+3]
			}
		}
	default:
		for y := 0; y < height; y++ {
			dst := pix[y*stride : y*stride+width*4]
			for x := 0; x < width; x++ {
				r, g, b, a := picture.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
				dst[x*4+0] = uint8(b >> 8)
				dst[x*4+1] = uint8(g >> 8)
				dst[x*4+2] = uint8(r >> 8)
				dst[x*4+3] = uint8(a >> 8)
			}
		}
	}
	return ui.Bitmap{Pix: pix, Width: width, Height: height, Stride: stride, ID: 1, Version: 1}, nil
}

// paintPreviewImage 把已解码的位图按对话框内容区绘制。内容区几何来自上一
// 帧的布局（与节点列表同一套做法），首帧只显示加载文案。
func (a *App) paintPreviewImage(canvas *ui.Canvas) {
	if a.dialog != dialogPreview || a.preview.bitmap.Width <= 0 {
		return
	}
	area := a.previewArea.Rect()
	if area.Width < 1 || area.Height < 1 {
		return
	}
	components.PaintImageView(canvas, a.theme, area, components.ImageViewOptions{
		Bitmap: a.preview.bitmap,
		Zoom:   a.preview.zoom,
	})
}
