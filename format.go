package main

import "strconv"

type FileSize int64

func (fs FileSize) String() string {
	prefixes := [...]string{"byte(s)", "kB", "MB", "GB"}

	for i, prefix := range prefixes {
		divisor := int64(2 << (i * 10))
		if int64(fs) < divisor {
			return strconv.FormatFloat(float64(fs)/float64(divisor), 'f', 2, 64) + " " + prefix
		}
	}

	return strconv.FormatFloat(float64(fs)/float64(2<<((len(prefixes)-1)*10)), 'f', 2, 64) + " " + prefixes[len(prefixes)-1]
}
