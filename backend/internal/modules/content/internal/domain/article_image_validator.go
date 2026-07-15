package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
)

const (
	// MaxArticleImageBytes 是输入图片原始字节上限。
	MaxArticleImageBytes = 5 * 1024 * 1024
	// MaxArticleImagePixels 是解码前允许的最大像素数。
	MaxArticleImagePixels int64 = 25_000_000
	// MaxArticleImageDimension 是任一边允许的最大长度。
	MaxArticleImageDimension = 8192
)

// ArticleImageValidationLimits 是不可变的图片内容安全限制。
type ArticleImageValidationLimits struct {
	maxFileBytes int64
	maxPixels    int64
	maxDimension int
}

// NewArticleImageValidationLimits 从已验证配置构造图片内容安全限制。
func NewArticleImageValidationLimits(maxFileBytes, maxPixels int64, maxDimension int) ArticleImageValidationLimits {
	return ArticleImageValidationLimits{maxFileBytes: maxFileBytes, maxPixels: maxPixels, maxDimension: maxDimension}
}

// DefaultArticleImageValidationLimits 返回兼容既有行为的默认安全限制。
func DefaultArticleImageValidationLimits() ArticleImageValidationLimits {
	return NewArticleImageValidationLimits(MaxArticleImageBytes, MaxArticleImagePixels, MaxArticleImageDimension)
}

var (
	ErrArticleImageTooLarge    = errors.New("图片超过文件大小限制")
	ErrArticleImageDimensions  = errors.New("图片尺寸超过限制")
	ErrUnsupportedArticleImage = errors.New("仅支持 JPEG 和 PNG 图片")
	ErrInvalidArticleImage     = errors.New("图片内容损坏或包含尾随数据")
)

// ValidatedArticleImage 是完整解码并清除元数据后的安全图片。
type ValidatedArticleImage struct {
	Bytes  []byte
	SHA256 string
	// MediaType 是服务端确认的媒体类型。
	MediaType MediaType
	Width     int
	Height    int
}

// ValidateArticleImage 使用既有默认限制读取、完整解码并重新编码 JPEG 或 PNG。
func ValidateArticleImage(reader io.Reader) (ValidatedArticleImage, error) {
	return ValidateArticleImageWithLimits(reader, DefaultArticleImageValidationLimits())
}

// ValidateArticleImageWithLimits 使用注入限制读取、完整解码并重新编码 JPEG 或 PNG。
func ValidateArticleImageWithLimits(reader io.Reader, limits ArticleImageValidationLimits) (ValidatedArticleImage, error) {
	limited := io.LimitReader(reader, limits.maxFileBytes+1)
	source, err := io.ReadAll(limited)
	if err != nil {
		return ValidatedArticleImage{}, fmt.Errorf("读取图片：%w", err)
	}
	if int64(len(source)) > limits.maxFileBytes {
		return ValidatedArticleImage{}, ErrArticleImageTooLarge
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(source))
	if err != nil {
		if bytes.HasPrefix(source, []byte{0xff, 0xd8}) || bytes.HasPrefix(source, []byte{0x89, 'P', 'N', 'G'}) {
			return ValidatedArticleImage{}, ErrInvalidArticleImage
		}
		return ValidatedArticleImage{}, ErrUnsupportedArticleImage
	}
	if format != "jpeg" && format != "png" {
		return ValidatedArticleImage{}, ErrUnsupportedArticleImage
	}
	pixels := int64(config.Width) * int64(config.Height)
	if config.Width < 1 || config.Height < 1 || config.Width > limits.maxDimension || config.Height > limits.maxDimension || pixels > limits.maxPixels {
		return ValidatedArticleImage{}, ErrArticleImageDimensions
	}
	if !hasExactImageEnding(source, format) {
		return ValidatedArticleImage{}, ErrInvalidArticleImage
	}
	decoded, decodedFormat, err := image.Decode(bytes.NewReader(source))
	if err != nil || decodedFormat != format {
		return ValidatedArticleImage{}, ErrInvalidArticleImage
	}
	var output bytes.Buffer
	var mediaType MediaType
	switch format {
	case "jpeg":
		mediaType = MediaTypeJPEG
		err = jpeg.Encode(&output, decoded, &jpeg.Options{Quality: 90})
	case "png":
		mediaType = MediaTypePNG
		err = png.Encode(&output, decoded)
	default:
		return ValidatedArticleImage{}, ErrUnsupportedArticleImage
	}
	if err != nil {
		return ValidatedArticleImage{}, fmt.Errorf("重新编码图片：%w", err)
	}
	payload := output.Bytes()
	digest := sha256.Sum256(payload)
	return ValidatedArticleImage{Bytes: append([]byte(nil), payload...), SHA256: hex.EncodeToString(digest[:]), MediaType: mediaType, Width: config.Width, Height: config.Height}, nil
}

func hasExactImageEnding(source []byte, format string) bool {
	switch format {
	case "jpeg":
		length, err := jpegEncodedLength(source)
		return err == nil && length == len(source)
	case "png":
		return len(source) >= 12 && bytes.Equal(source[len(source)-12:], []byte{0, 0, 0, 0, 'I', 'E', 'N', 'D', 0xae, 0x42, 0x60, 0x82})
	default:
		return false
	}
}
