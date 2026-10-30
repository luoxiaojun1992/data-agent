package knowledge

import "testing"

func TestIsImage(t *testing.T) {
	cases := []struct {
		fileType string
		want     bool
	}{
		{FileTypeImage, true}, // 精确匹配 image
		{"image", true},
		{FileTypeTxt, false},
		{"txt", false},
		{"png", false}, // 具体图片扩展名不算（legacy pdf/docx 等也不走多模态路径）
		{"", false},
	}
	for _, c := range cases {
		if got := IsImage(c.fileType); got != c.want {
			t.Errorf("IsImage(%q) = %v, want %v", c.fileType, got, c.want)
		}
	}
}
