package content

// ifNoneMatch 按 RFC 9110 的弱比较语义扫描 If-None-Match。
func ifNoneMatch(header, current string) bool {
	currentOpaque, valid := singleEntityTag(current)
	if !valid {
		return false
	}
	index := skipOWS(header, 0)
	if index < len(header) && header[index] == '*' {
		return skipOWS(header, index+1) == len(header)
	}
	matched, seen := false, false
	for index < len(header) {
		opaque, next, tagValid := scanEntityTag(header, index)
		if !tagValid {
			return false
		}
		seen = true
		matched = matched || opaque == currentOpaque
		index = skipOWS(header, next)
		if index == len(header) {
			return seen && matched
		}
		if header[index] != ',' {
			return false
		}
		index = skipOWS(header, index+1)
		if index == len(header) {
			return false
		}
	}
	return false
}

func singleEntityTag(value string) (string, bool) {
	opaque, next, valid := scanEntityTag(value, 0)
	return opaque, valid && skipOWS(value, next) == len(value)
}
func scanEntityTag(value string, index int) (string, int, bool) {
	index = skipOWS(value, index)
	if index+2 <= len(value) && value[index:index+2] == "W/" {
		index += 2
	}
	if index >= len(value) || value[index] != '"' {
		return "", index, false
	}
	index++
	start := index
	for index < len(value) && value[index] != '"' {
		character := value[index]
		if character < 0x21 || character == 0x7f {
			return "", index, false
		}
		index++
	}
	if index >= len(value) {
		return "", index, false
	}
	return value[start:index], index + 1, true
}
func skipOWS(value string, index int) int {
	for index < len(value) && (value[index] == ' ' || value[index] == '\t') {
		index++
	}
	return index
}
