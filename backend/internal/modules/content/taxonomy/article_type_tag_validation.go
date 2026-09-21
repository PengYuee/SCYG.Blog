package taxonomy

import (
	"math"
	"strings"
	"unicode"
	"unicode/utf8"
)

func parseID(value int64, field string) error {
	if value <= 0 {
		return invalid(field)
	}
	return nil
}

func parseVersion(value uint64) error {
	if value == 0 {
		return invalid("version")
	}
	return nil
}

func parseName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || !utf8.ValidString(value) || len([]rune(value)) > 60 {
		return "", invalid("name")
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return "", invalid("name")
		}
	}
	return value, nil
}

func parseImage(value *string) (*string, error) { //nolint:nilnil // nil is the valid representation of an omitted image.
	if value == nil {
		return nil, nil //nolint:nilnil // nil is the valid representation of an omitted image.
	}
	if !utf8.ValidString(*value) || len([]rune(*value)) > 512 {
		return nil, invalid("image")
	}
	for _, character := range *value {
		if unicode.IsControl(character) {
			return nil, invalid("image")
		}
	}
	copy := *value
	return &copy, nil
}

func parseMeun(value int32) error {
	if value < 0 || int64(value) > math.MaxInt16 {
		return invalid("meun")
	}
	return nil
}
