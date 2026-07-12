package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"testing"
)

func Test_ValidateArticleImage_rejects_jpeg_payload_followed_by_fake_eoi(t *testing.T) {
	var source bytes.Buffer
	if err := jpeg.Encode(&source, fixtureImage(2, 2), nil); err != nil {
		t.Fatal(err)
	}
	attack := append(append([]byte(nil), source.Bytes()...), []byte("dangerous-polyglot")...)
	attack = append(attack, 0xff, 0xd9)
	if _, err := ValidateArticleImage(bytes.NewReader(attack)); !errors.Is(err, ErrInvalidArticleImage) {
		t.Fatalf("期望拒绝 JPEG polyglot，实际 %v", err)
	}
}

func Test_ValidateArticleImage_accepts_exact_input_and_dimension_boundaries(t *testing.T) {
	tests := []struct {
		name          string
		input         func(*testing.T) []byte
		width, height int
	}{
		{"exact 5MiB JPEG", exactSizeJPEG, 1, 1},
		{"exact 8192 dimension PNG", func(t *testing.T) []byte { return encodedPNG(t, MaxArticleImageDimension, 1) }, MaxArticleImageDimension, 1},
		{"exact 25MP PNG", func(t *testing.T) []byte { return encodedPNG(t, 5000, 5000) }, 5000, 5000},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := ValidateArticleImage(bytes.NewReader(test.input(t)))
			if err != nil {
				t.Fatalf("边界输入被拒绝：%v", err)
			}
			if result.Width != test.width || result.Height != test.height {
				t.Fatalf("尺寸=%dx%d", result.Width, result.Height)
			}
		})
	}
}

func Test_ValidateArticleImage_output_decodes_and_hashes_final_bytes(t *testing.T) {
	source := encodedPNG(t, 3, 2)
	result, err := ValidateArticleImage(bytes.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}
	decoded, format, err := image.Decode(bytes.NewReader(result.Bytes))
	if err != nil {
		t.Fatalf("最终字节不可解码：%v", err)
	}
	digest := sha256.Sum256(result.Bytes)
	if format != "png" || decoded.Bounds().Dx() != 3 || decoded.Bounds().Dy() != 2 || result.SHA256 != hex.EncodeToString(digest[:]) {
		t.Fatalf("最终输出契约不一致")
	}
}

func exactSizeJPEG(t *testing.T) []byte {
	t.Helper()
	var base bytes.Buffer
	if err := jpeg.Encode(&base, fixtureImage(1, 1), nil); err != nil {
		t.Fatal(err)
	}
	remaining := MaxArticleImageBytes - base.Len()
	output := append([]byte(nil), base.Bytes()[:2]...)
	for remaining > 0 {
		payload := remaining - 4
		if payload > 65533 {
			payload = 65533
		}
		if payload < 0 {
			t.Fatalf("无法构造精确 JPEG")
		}
		segment := make([]byte, payload+4)
		segment[0], segment[1] = 0xff, 0xe2
		binary.BigEndian.PutUint16(segment[2:4], uint16(payload+2))
		output = append(output, segment...)
		remaining -= len(segment)
	}
	output = append(output, base.Bytes()[2:]...)
	if len(output) != MaxArticleImageBytes {
		t.Fatalf("fixture bytes=%d", len(output))
	}
	return output
}

func encodedPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	var output bytes.Buffer
	if err := png.Encode(&output, image.NewNRGBA(image.Rect(0, 0, width, height))); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
