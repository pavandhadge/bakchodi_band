package client

import (
	"bufio"
	crand "crypto/rand"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pavandhadge/dopamine_blocker/internal/model"
)

func ApplyResistance(targetType, target string, policy model.FrictionPolicy) {
	waitSeconds, targetName, ok := resistanceTarget(targetType, target)
	if !ok {
		return
	}
	waitSeconds += policy.ExtraWait

	fmt.Printf("\nYou're about to unlock %s\n", targetName)
	if policy.AttemptsToday > 0 {
		fmt.Printf("Escalation: %d unlock attempt(s) today, adding %d seconds and %d challenge(s).\n", policy.AttemptsToday, policy.ExtraWait, policy.Challenges)
	}
	fmt.Printf("Wait %d seconds to confirm you really want this...\n\n", waitSeconds)
	for i := waitSeconds; i > 0; i-- {
		fmt.Printf("\rTime remaining: %2d seconds ", i)
		time.Sleep(1 * time.Second)
	}
	fmt.Println("\n\nTime's up.")

	requiredCorrect := policy.Challenges
	if requiredCorrect < 3 {
		requiredCorrect = 3
	}
	maxMistakes := 3

	reader := bufio.NewReader(os.Stdin)
	mistakes := 0
	for solved := 0; solved < requiredCorrect; {
		question, answer := generateMathChallenge()
		fmt.Printf("\nChallenge %d/%d: %s = ? ", solved+1, requiredCorrect, question)

		input, _ := reader.ReadString('\n')
		userAnswer, err := strconv.Atoi(strings.TrimSpace(input))
		if err != nil {
			mistakes++
			fmt.Printf("Invalid number. Mistake %d/%d.\n", mistakes, maxMistakes)
			if mistakes >= maxMistakes {
				break
			}
			continue
		}

		if userAnswer == answer {
			solved++
			fmt.Printf("Correct. %d more required.\n", requiredCorrect-solved)
			continue
		}

		mistakes++
		fmt.Printf("Wrong. Mistake %d/%d.\n", mistakes, maxMistakes)
		if mistakes >= maxMistakes {
			break
		}
	}

	if mistakes < maxMistakes {
		fmt.Println("\nAll challenges solved. Proceeding with unlock...")
		return
	}

	fmt.Println("\nToo many failed attempts. Unlock cancelled.")
	os.Exit(0)
}

func ApplyBreakGlassResistance(targetType, target string, policy model.FrictionPolicy) {
	waitSeconds, targetName, ok := resistanceTarget(targetType, target)
	if !ok {
		return
	}
	waitSeconds *= 3

	fmt.Printf("\nBREAK GLASS unlock requested for %s\n", targetName)
	fmt.Printf("This bypasses normal budget/commitment checks, is limited to %d minutes, and is logged.\n", 5)
	fmt.Printf("Wait %d seconds and solve all challenges to continue.\n\n", waitSeconds)
	for i := waitSeconds; i > 0; i-- {
		fmt.Printf("\rTime remaining: %2d seconds ", i)
		time.Sleep(1 * time.Second)
	}
	fmt.Println("\n\nBreak-glass delay complete.")
	policy.ExtraWait += 120
	policy.Challenges += 2
	ApplyResistance(targetType, target, policy)
}

func resistanceTarget(targetType, target string) (int, string, bool) {
	switch targetType {
	case "unlock":
		return 60, "all sites", true
	case "unlock-group":
		return 60, "group: " + target, true
	case "unlock-url":
		return 30, "URL: " + target, true
	default:
		return 0, "", false
	}
}

func generateMathChallenge() (string, int) {
	a := randomInt(37, 96)
	b := randomInt(12, 39)
	c := randomInt(120, 460)
	d := randomInt(7, 28)
	e := randomInt(4, 17)

	switch randomInt(0, 2) {
	case 0:
		return fmt.Sprintf("(%d x %d) - %d + (%d x %d)", a, b, c, d, e), (a*b - c) + (d * e)
	case 1:
		return fmt.Sprintf("(%d + %d) x %d - %d", a, c, e, b*d), (a+c)*e - (b * d)
	default:
		return fmt.Sprintf("(%d x %d) + %d - (%d x %d)", c, e, a, b, d), (c*e + a) - (b * d)
	}
}

func randomInt(min, max int) int {
	if max < min {
		min, max = max, min
	}
	n, err := crand.Int(crand.Reader, big.NewInt(int64(max-min+1)))
	if err != nil {
		return min
	}
	return min + int(n.Int64())
}
