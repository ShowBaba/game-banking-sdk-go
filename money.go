package gamebank

import (
	"fmt"
	"strconv"
	"strings"
)

// ToMinor converts a decimal string to minor units: ToMinor("100.50", 2) is 10050.
// It fails on more decimal places than the exponent allows or on overflow.
// It never uses floating point.
func ToMinor(amount string, exponent int) (int64, error) {
	if exponent < 0 || exponent > 6 {
		return 0, fmt.Errorf("gamebank: exponent must be 0 to 6")
	}
	s := strings.ReplaceAll(strings.TrimSpace(amount), ",", "")
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	whole, frac, hasDot := strings.Cut(s, ".")
	if whole == "" || (hasDot && frac == "") || !digits(whole) || !digits(frac) {
		return 0, fmt.Errorf("gamebank: not a decimal amount: %q", amount)
	}
	if len(frac) > exponent {
		return 0, fmt.Errorf("gamebank: %s has more than %d decimal places", amount, exponent)
	}
	n, err := strconv.ParseInt(whole+frac+strings.Repeat("0", exponent-len(frac)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("gamebank: %s is too large", amount)
	}
	if neg {
		n = -n
	}
	return n, nil
}

// FormatMinor formats minor units as a plain decimal string: FormatMinor(10050, 2) is "100.50".
func FormatMinor(minor int64, exponent int) string {
	if exponent < 0 || exponent > 6 {
		exponent = 0
	}
	sign := ""
	var u uint64
	if minor < 0 {
		sign = "-"
		u = uint64(-(minor + 1)) + 1 // safe for math.MinInt64
	} else {
		u = uint64(minor)
	}
	s := strconv.FormatUint(u, 10)
	if exponent == 0 {
		return sign + s
	}
	if len(s) <= exponent {
		s = strings.Repeat("0", exponent-len(s)+1) + s
	}
	return sign + s[:len(s)-exponent] + "." + s[len(s)-exponent:]
}

func digits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
