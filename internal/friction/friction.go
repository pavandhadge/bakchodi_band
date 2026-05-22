package friction

import (
	crand "crypto/rand"
	"fmt"
	"math/big"
)

const (
	MinChallenges = 3
	MaxMistakes   = 3
)

type TargetType int

const (
	TargetAll   TargetType = iota
	TargetGroup
	TargetURL
)

type TargetInfo struct {
	WaitSeconds int
	Name        string
	OK          bool
}

func ResistanceTarget(targetType TargetType, value string) TargetInfo {
	switch targetType {
	case TargetAll:
		return TargetInfo{WaitSeconds: 60, Name: "all sites", OK: true}
	case TargetGroup:
		return TargetInfo{WaitSeconds: 60, Name: "group: " + value, OK: true}
	case TargetURL:
		return TargetInfo{WaitSeconds: 30, Name: "URL: " + value, OK: true}
	default:
		return TargetInfo{OK: false}
	}
}

func ParseTargetType(t string) TargetType {
	switch t {
	case "unlock", "lock":
		return TargetAll
	case "unlock-group", "lock-group":
		return TargetGroup
	case "unlock-url", "lock-url":
		return TargetURL
	default:
		return TargetAll
	}
}

func GenerateMathChallenge() (string, int) {
	a := RandomInt(37, 96)
	b := RandomInt(12, 39)
	c := RandomInt(120, 460)
	d := RandomInt(7, 28)
	e := RandomInt(4, 17)

	switch RandomInt(0, 2) {
	case 0:
		return fmt.Sprintf("(%d x %d) - %d + (%d x %d)", a, b, c, d, e), (a*b - c) + (d * e)
	case 1:
		return fmt.Sprintf("(%d + %d) x %d - %d", a, c, e, b*d), (a+c)*e - (b * d)
	default:
		return fmt.Sprintf("(%d x %d) + %d - (%d x %d)", c, e, a, b, d), (c*e + a) - (b * d)
	}
}

func RandomInt(min, max int) int {
	if max < min {
		min, max = max, min
	}
	n, err := crand.Int(crand.Reader, big.NewInt(int64(max-min+1)))
	if err != nil {
		return min
	}
	return min + int(n.Int64())
}
