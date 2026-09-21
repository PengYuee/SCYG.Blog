package content

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"os"

	"github.com/gin-gonic/gin"

	generated "github.com/PengYuee/SCYG.Blog/backend/internal/generated/openapi"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/image"
)

type (
	requestTempHandle interface {
		io.Reader
		io.Writer
		Sync() error
		Close() error
		Chmod(os.FileMode) error
		Name() string
	}
	requestTempOperations interface {
		Create() (requestTempHandle, error)
		Open(string) (requestTempHandle, error)
		Remove(string) error
	}
	requestImageFile struct {
		file       requestTempHandle
		path       string
		operations requestTempOperations
	}
)

func (source *requestImageFile) Read(buffer []byte) (int, error) {
	return source.file.Read(buffer)
}

func (source *requestImageFile) ReadArticleImage(buffer []byte) (int, error) {
	return source.Read(buffer)
}

func (source *requestImageFile) closeAndRemove() error {
	return errors.Join(source.file.Close(), source.operations.Remove(source.path))
}

type osRequestTempOperations struct{}

func (osRequestTempOperations) Create() (requestTempHandle, error) {
	return os.CreateTemp("", "scyg-article-image-request-*.tmp")
}

//nolint:gosec // path is created and owned by the request temporary-file operations.
func (osRequestTempOperations) Open(path string) (requestTempHandle, error) { return os.Open(path) }

func (osRequestTempOperations) Remove(path string) error { return os.Remove(path) }

type contextPartReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader contextPartReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(buffer)
}

// CreateManageArticleImage accepts and stores one managed article image upload.
func (handler *Handler) CreateManageArticleImage(ctx context.Context, request generated.CreateManageArticleImageRequestObject) (generated.CreateManageArticleImageResponseObject, error) {
	service := handler.images
	source, err := handler.spoolUniqueImagePart(requestContext(ctx), request.Body)
	if err != nil {
		return nil, newRESTError(codeValidation, err)
	}
	cleaned := false
	defer func() {
		if !cleaned {
			_ = source.closeAndRemove()
		}
	}()
	result, uploadErr := service.Upload(requestContext(ctx), image.Upload{Content: source})
	cleanupErr := source.closeAndRemove()
	cleaned = true
	if cleanupErr != nil {
		return nil, fmt.Errorf("清理图片请求临时文件：%w", cleanupErr)
	}
	if uploadErr != nil {
		return nil, uploadErr
	}
	width, err := generatedImageDimension(result.Width)
	if err != nil {
		return nil, err
	}
	height, err := generatedImageDimension(result.Height)
	if err != nil {
		return nil, err
	}
	dto := generated.ArticleImage{ID: result.ID, StorageKey: result.StorageKey, URL: result.URL, MediaType: generated.ArticleImageMediaType(mediaLabel(result.MediaType)), ByteSize: result.ByteSize, Width: width, Height: height, Status: generated.ArticleImageStatus(result.Status), ExpiresAt: result.ExpiresAt}
	return generated.CreateManageArticleImage201JSONResponse{Body: dto, Headers: generated.CreateManageArticleImage201ResponseHeaders{Location: result.URL}}, nil
}

// DeleteArticleImage cancels one pending article image.
func (handler *Handler) DeleteArticleImage(ctx context.Context, request generated.DeleteArticleImageRequestObject) (generated.DeleteArticleImageResponseObject, error) {
	service := handler.images
	if err := service.Cancel(requestContext(ctx), image.Delete{ID: request.ImageID}); err != nil {
		return nil, err
	}
	return generated.DeleteArticleImage204Response{}, nil
}

// GetArticleImageMedia serves one article image Blob.
func (handler *Handler) GetArticleImageMedia(ctx context.Context, request generated.GetArticleImageMediaRequestObject) (generated.GetArticleImageMediaResponseObject, error) {
	service := handler.images
	media, err := service.GetMedia(requestContext(ctx), image.Get{StorageKey: request.StorageKey})
	if err != nil {
		return nil, err
	}
	tag := "\"" + media.SHA256 + "\""
	ginContext, _ := ctx.(*gin.Context)
	if ginContext != nil {
		ginContext.Header("X-Content-Type-Options", "nosniff")
		ginContext.Header("Content-Disposition", fmt.Sprintf("inline; filename=%q", request.StorageKey))
		if media.Pending {
			ginContext.Header("Cache-Control", "private, no-store")
		} else {
			ginContext.Header("Cache-Control", "public, max-age=31536000, immutable")
		}
		if ifNoneMatch(ginContext.GetHeader("If-None-Match"), tag) {
			return generated.GetArticleImageMedia304Response{Headers: generated.GetArticleImageMedia304ResponseHeaders{ETag: tag}}, nil
		}
	}
	headers := generated.GetArticleImageMedia200ResponseHeaders{ETag: tag}
	if media.MediaType == "image/png" {
		return generated.GetArticleImageMedia200ImagePngResponse{Body: bytes.NewReader(media.Content), Headers: headers, ContentLength: media.ByteSize}, nil
	}
	return generated.GetArticleImageMedia200ImageJpegResponse{Body: bytes.NewReader(media.Content), Headers: headers, ContentLength: media.ByteSize}, nil
}

func generatedImageDimension(value int) (int32, error) {
	if value < 0 || value > math.MaxInt32 {
		return 0, responseMappingError()
	}
	return int32(value), nil
}

func (handler *Handler) spoolUniqueImagePart(ctx context.Context, reader *multipart.Reader) (source *requestImageFile, err error) {
	if reader == nil {
		return nil, errors.New("缺少 multipart 请求体")
	}
	part, err := reader.NextPart()
	if errors.Is(err, io.EOF) {
		return nil, errors.New("缺少 file part")
	}
	if err != nil {
		return nil, fmt.Errorf("读取 multipart：%w", err)
	}
	defer func() { _ = part.Close() }()
	if part.FormName() != "file" {
		return nil, errors.New("multipart 只能包含一个 file part")
	}
	temporary, err := handler.tempFiles.Create()
	if err != nil {
		return nil, fmt.Errorf("创建请求临时文件：%w", err)
	}
	path := temporary.Name()
	if err = temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		_ = handler.tempFiles.Remove(path)
		return nil, fmt.Errorf("限制请求临时文件权限：%w", err)
	}
	cleanup := func() { _ = temporary.Close(); _ = handler.tempFiles.Remove(path) }
	written, copyErr := io.Copy(temporary, io.LimitReader(contextPartReader{ctx: ctx, reader: part}, handler.imagePolicy.MaxFileBytes()+1))
	if copyErr != nil {
		cleanup()
		return nil, fmt.Errorf("暂存图片请求：%w", copyErr)
	}
	if written > handler.imagePolicy.MaxFileBytes() {
		cleanup()
		return nil, fmt.Errorf("图片超过 %d 字节限制", handler.imagePolicy.MaxFileBytes())
	}
	if err = temporary.Sync(); err != nil {
		cleanup()
		return nil, fmt.Errorf("同步请求临时文件：%w", err)
	}
	if err = temporary.Close(); err != nil {
		_ = handler.tempFiles.Remove(path)
		return nil, fmt.Errorf("关闭请求临时文件：%w", err)
	}
	if next, nextErr := reader.NextPart(); nextErr == nil {
		_ = next.Close()
		_ = handler.tempFiles.Remove(path)
		return nil, errors.New("multipart 只能包含一个 file part")
	} else if !errors.Is(nextErr, io.EOF) {
		_ = handler.tempFiles.Remove(path)
		return nil, fmt.Errorf("读取 multipart：%w", nextErr)
	}
	file, err := handler.tempFiles.Open(path)
	if err != nil {
		_ = handler.tempFiles.Remove(path)
		return nil, fmt.Errorf("打开请求临时文件：%w", err)
	}
	return &requestImageFile{file: file, path: path, operations: handler.tempFiles}, nil
}

func mediaLabel(value string) string {
	if value == "image/png" {
		return "png"
	}
	return "jpeg"
}

func requestContext(ctx context.Context) context.Context {
	if ginContext, ok := ctx.(*gin.Context); ok {
		return ginContext.Request.Context()
	}
	return ctx
}
