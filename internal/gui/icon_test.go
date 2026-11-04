package gui

import "testing"

// TestWindowIconIsEmbedded 防止图标文件丢失或格式变了没人发现：尺寸、通道数、
// 圆角外的透明度、方块内的不透明度都检查一遍。
func TestWindowIconIsEmbedded(t *testing.T) {
	icon := windowIcon()
	if icon.Size < 64 {
		t.Fatalf("窗口图标尺寸 = %d，期望至少 64", icon.Size)
	}
	if len(icon.RGBA) != icon.Size*icon.Size*4 {
		t.Fatalf("窗口图标字节数 = %d，期望 %d", len(icon.RGBA), icon.Size*icon.Size*4)
	}
	center := ((icon.Size/2)*icon.Size + icon.Size/2) * 4
	if icon.RGBA[center+3] != 255 {
		t.Fatalf("图标中心应当不透明，alpha = %d", icon.RGBA[center+3])
	}
	if icon.RGBA[3] != 0 {
		t.Fatalf("圆角外应当透明，alpha = %d", icon.RGBA[3])
	}
	top := ((icon.Size / 10) * icon.Size) + icon.Size/2
	if icon.RGBA[top*4+3] != 255 {
		t.Fatalf("方块上缘中点应当不透明，alpha = %d", icon.RGBA[top*4+3])
	}
}

// TestBrandBitmapIsPremultiplied 验证标题栏位图按 Viva 约定是预乘 BGRA：
// 任一通道都不应超过 alpha。
func TestBrandBitmapIsPremultiplied(t *testing.T) {
	bitmap := brandBitmap()
	if bitmap.Width == 0 || bitmap.Height == 0 {
		t.Fatal("品牌图标没有解码出来")
	}
	if len(bitmap.Pix) != bitmap.Width*bitmap.Height*4 || bitmap.Stride != bitmap.Width*4 {
		t.Fatalf("品牌位图尺寸不一致：%dx%d stride=%d pix=%d", bitmap.Width, bitmap.Height, bitmap.Stride, len(bitmap.Pix))
	}
	for index := 0; index < len(bitmap.Pix); index += 4 {
		alpha := bitmap.Pix[index+3]
		for channel := 0; channel < 3; channel++ {
			if bitmap.Pix[index+channel] > alpha {
				t.Fatalf("通道值 %d 超过 alpha %d（未预乘）", bitmap.Pix[index+channel], alpha)
			}
		}
	}
}

// TestConfigCarriesWindowIcon 确认配置里真的带上了图标（否则任务栏是默认图标）。
func TestConfigCarriesWindowIcon(t *testing.T) {
	application, _ := openTestApp(t, t.TempDir(), newFakeRunner(nil))
	defer application.Close()
	if application.config.Icon.Size == 0 {
		t.Fatal("app.Config 没有带上窗口图标")
	}
}
