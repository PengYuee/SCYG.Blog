package domain

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

func Test_ValidateArticleImage_reencodes_jpeg_and_png(t *testing.T) {
	tests := []struct {
		name   string
		encode func(*bytes.Buffer) error
		media  MediaType
	}{
		{"jpeg", func(buffer *bytes.Buffer) error {
			return jpeg.Encode(buffer, fixtureImage(8, 6), &jpeg.Options{Quality: 70})
		}, MediaTypeJPEG},
		{"png", func(buffer *bytes.Buffer) error { return png.Encode(buffer, fixtureImage(8, 6)) }, MediaTypePNG},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var source bytes.Buffer
			if err := test.encode(&source); err != nil {
				t.Fatal(err)
			}
			result, err := ValidateArticleImage(bytes.NewReader(source.Bytes()))
			if err != nil {
				t.Fatalf("校验失败：%v", err)
			}
			if result.MediaType != test.media || result.Width != 8 || result.Height != 6 || len(result.SHA256) != 64 || len(result.Bytes) == 0 {
				t.Fatalf("结果不合法：%+v", result)
			}
		})
	}
}

func Test_ValidateArticleImage_rejects_unsupported_corrupt_and_oversized(t *testing.T) {
	var gifBytes bytes.Buffer
	_ = gif.Encode(&gifBytes, fixtureImage(1, 1), nil)
	var jpegBytes bytes.Buffer
	_ = jpeg.Encode(&jpegBytes, fixtureImage(2, 2), nil)
	truncated := jpegBytes.Bytes()[:len(jpegBytes.Bytes())/2]
	tests := []struct {
		name   string
		input  []byte
		target error
	}{
		{"gif", gifBytes.Bytes(), ErrUnsupportedArticleImage},
		{"svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), ErrUnsupportedArticleImage},
		{"truncated", truncated, ErrInvalidArticleImage},
		{"5MiB plus one", bytes.Repeat([]byte{0}, MaxArticleImageBytes+1), ErrArticleImageTooLarge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ValidateArticleImage(bytes.NewReader(test.input))
			if !errors.Is(err, test.target) {
				t.Fatalf("期望 %v，实际 %v", test.target, err)
			}
		})
	}
}

func Test_ValidateArticleImage_rejects_dimension_and_pixel_limits_before_decode(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
	}{{"dimension", MaxArticleImageDimension + 1, 1}, {"pixels", 5001, 5000}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var source bytes.Buffer
			if err := png.Encode(&source, image.NewNRGBA(image.Rect(0, 0, test.width, test.height))); err != nil {
				t.Fatal(err)
			}
			_, err := ValidateArticleImage(bytes.NewReader(source.Bytes()))
			if !errors.Is(err, ErrArticleImageDimensions) {
				t.Fatalf("期望尺寸错误，实际 %v", err)
			}
		})
	}
}

func Test_ValidateArticleImage_rejects_nonempty_trailing_payload(t *testing.T) {
	var source bytes.Buffer
	_ = png.Encode(&source, fixtureImage(1, 1))
	source.WriteString(strings.Repeat("X", 8))
	if _, err := ValidateArticleImage(bytes.NewReader(source.Bytes())); !errors.Is(err, ErrInvalidArticleImage) {
		t.Fatalf("期望尾随数据错误，实际 %v", err)
	}
}

func fixtureImage(width, height int) image.Image {
	value := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			value.Set(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 80, A: 255})
		}
	}
	return value
}
