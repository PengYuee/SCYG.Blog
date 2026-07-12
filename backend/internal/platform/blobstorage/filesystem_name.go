package blobstorage

import (
	"context"
	"errors"
	"io"
	"strings"
)

func copyContext(ctx context.Context, destination io.Writer, source io.Reader) (int64, error) {
	buffer := make([]byte, 32*1024)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		count, readErr := source.Read(buffer)
		if count > 0 {
			written, writeErr := destination.Write(buffer[:count])
			total += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
			if written != count {
				return total, io.ErrShortWrite
			}
		}
		if errors.Is(readErr, io.EOF) {
			return total, nil
		}
		if readErr != nil {
			return total, readErr
		}
	}
}
func validKey(value string) bool {
	return len(value) == 36 && lowerHex(value[:32], 32) && value[32] == '.' && (value[33:] == "jpg" || value[33:] == "png")
}
func validTemp(value string) bool {
	return len(value) == len(tempPrefix)+32+1+24+4 && strings.HasPrefix(value, tempPrefix) && lowerHex(value[len(tempPrefix):len(tempPrefix)+32], 32) && value[len(tempPrefix)+32] == '-' && lowerHex(value[len(tempPrefix)+33:len(tempPrefix)+57], 24) && strings.HasSuffix(value, ".tmp")
}
func lowerHex(value string, size int) bool {
	if len(value) != size {
		return false
	}
	for _, char := range value {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
			return false
		}
	}
	return true
}
