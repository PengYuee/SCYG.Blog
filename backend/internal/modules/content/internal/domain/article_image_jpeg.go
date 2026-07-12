package domain

// jpegEncodedLength 返回第一个真实 EOI 标记结束位置，熵编码中的转义与重启标记不会被误判。
func jpegEncodedLength(source []byte) (int, error) {
	if len(source) < 4 || source[0] != 0xff || source[1] != 0xd8 {
		return 0, ErrInvalidArticleImage
	}
	position := 2
	inScan := false
	for position < len(source) {
		if inScan {
			for position < len(source) && source[position] != 0xff {
				position++
			}
			if position >= len(source) {
				return 0, ErrInvalidArticleImage
			}
		} else if source[position] != 0xff {
			return 0, ErrInvalidArticleImage
		}
		for position < len(source) && source[position] == 0xff {
			position++
		}
		if position >= len(source) {
			return 0, ErrInvalidArticleImage
		}
		marker := source[position]
		position++
		if inScan {
			switch {
			case marker == 0x00, marker >= 0xd0 && marker <= 0xd7:
				continue
			default:
				inScan = false
			}
		}
		switch {
		case marker == 0xd9:
			return position, nil
		case marker == 0xd8 || marker == 0x00:
			return 0, ErrInvalidArticleImage
		case marker == 0x01, marker >= 0xd0 && marker <= 0xd7:
			continue
		}
		if position+2 > len(source) {
			return 0, ErrInvalidArticleImage
		}
		segmentLength := int(source[position])<<8 | int(source[position+1])
		if segmentLength < 2 || position+segmentLength > len(source) {
			return 0, ErrInvalidArticleImage
		}
		position += segmentLength
		inScan = marker == 0xda
	}
	return 0, ErrInvalidArticleImage
}
