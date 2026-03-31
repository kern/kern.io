package crdt

// Fractional indexing for ordering siblings within a parent.
//
// Positions are strings that can be compared lexicographically.
// We use a base-62 character set (0-9, A-Z, a-z) so positions
// are compact and always allow insertion between any two positions.

const posChars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
const posBase = len(posChars) // 62

// PositionFirst returns the first default position.
func PositionFirst() string {
	return "V" // midpoint of the character set
}

// PositionBetween generates a position string between a and b.
// If a is empty, generates before b. If b is empty, generates after a.
// If both empty, returns the midpoint.
func PositionBetween(a, b string) string {
	if a == "" && b == "" {
		return "V"
	}
	if a == "" {
		return positionBefore(b)
	}
	if b == "" {
		return positionAfter(a)
	}
	return positionMid(a, b)
}

// positionAfter generates a position after a.
func positionAfter(a string) string {
	// Try incrementing the last character
	runes := []byte(a)
	for i := len(runes) - 1; i >= 0; i-- {
		idx := charIndex(runes[i])
		if idx < posBase-1 {
			runes[i] = posChars[idx+1]
			return string(runes[:i+1])
		}
	}
	// All chars maxed out, append midpoint
	return a + string(posChars[posBase/2])
}

// positionBefore generates a position before b.
func positionBefore(b string) string {
	runes := []byte(b)
	for i := len(runes) - 1; i >= 0; i-- {
		idx := charIndex(runes[i])
		if idx > 0 {
			runes[i] = posChars[idx-1]
			return string(runes[:i+1])
		}
	}
	// All chars at minimum, prepend and use midpoint
	return string(posChars[0]) + string(posChars[posBase/2])
}

// positionMid generates a position between a and b (a < b lexicographically).
func positionMid(a, b string) string {
	// Pad to same length
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}

	aIdx := make([]int, maxLen)
	bIdx := make([]int, maxLen)
	for i := 0; i < maxLen; i++ {
		if i < len(a) {
			aIdx[i] = charIndex(a[i])
		}
		if i < len(b) {
			bIdx[i] = charIndex(b[i])
		} else {
			bIdx[i] = posBase // treat missing chars in b as "after everything"
		}
	}

	// Find midpoint digit by digit
	result := make([]byte, 0, maxLen+1)
	for i := 0; i < maxLen; i++ {
		if aIdx[i] == bIdx[i] {
			result = append(result, posChars[aIdx[i]])
			continue
		}

		mid := (aIdx[i] + bIdx[i]) / 2
		if mid > aIdx[i] {
			result = append(result, posChars[mid])
			return string(result)
		}

		// No room at this digit, carry down
		result = append(result, posChars[aIdx[i]])
		// Next digit: between aIdx[i+1] (or 0) and posBase
		nextA := 0
		if i+1 < len(a) {
			nextA = charIndex(a[i+1])
		}
		mid = (nextA + posBase) / 2
		if mid > nextA {
			result = append(result, posChars[mid])
			return string(result)
		}
		// Keep going deeper
	}

	// Append midpoint character
	result = append(result, posChars[posBase/2])
	return string(result)
}

func charIndex(c byte) int {
	for i := 0; i < posBase; i++ {
		if posChars[i] == c {
			return i
		}
	}
	return 0
}

// PositionInitial generates n evenly-spaced initial positions.
func PositionInitial(n int) []string {
	if n <= 0 {
		return nil
	}
	result := make([]string, n)
	step := posBase / (n + 1)
	if step < 1 {
		step = 1
	}
	for i := 0; i < n; i++ {
		idx := step * (i + 1)
		if idx >= posBase {
			idx = posBase - 1
		}
		result[i] = string(posChars[idx])
	}
	return result
}
