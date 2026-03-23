package parser

func ExtractBlueprintStrings(data []byte) []string {
	results := make([]string, 0)
	seen := make(map[string]struct{})
	start := -1

	flush := func(end int) {
		if start < 0 {
			return
		}

		run := data[start:end]
		start = -1

		if len(run) < 32 || run[0] != '0' {
			return
		}

		candidate := string(run)
		if _, exists := seen[candidate]; exists {
			return
		}

		if _, err := decodeBlueprintString(candidate); err != nil {
			return
		}

		seen[candidate] = struct{}{}
		results = append(results, candidate)
	}

	for i, b := range data {
		if isBase64Byte(b) {
			if start < 0 {
				start = i
			}
			continue
		}

		flush(i)
	}

	flush(len(data))
	return results
}

func isBase64Byte(b byte) bool {
	switch {
	case b >= 'A' && b <= 'Z':
		return true
	case b >= 'a' && b <= 'z':
		return true
	case b >= '0' && b <= '9':
		return true
	case b == '+', b == '/', b == '=':
		return true
	default:
		return false
	}
}
