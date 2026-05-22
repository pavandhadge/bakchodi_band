package client

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pavandhadge/dopamine_blocker/internal/friction"
	"github.com/pavandhadge/dopamine_blocker/internal/model"
)

func ApplyResistance(targetType friction.TargetType, target string, policy model.FrictionPolicy) {
	info := friction.ResistanceTarget(targetType, target)
	if !info.OK {
		return
	}
	waitSeconds := info.WaitSeconds + policy.ExtraWait

	fmt.Printf("\nYou're about to unlock %s\n", info.Name)
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
	if requiredCorrect < friction.MinChallenges {
		requiredCorrect = friction.MinChallenges
	}
	maxMistakes := friction.MaxMistakes

	reader := bufio.NewReader(os.Stdin)
	mistakes := 0
	for solved := 0; solved < requiredCorrect; {
		question, answer := friction.GenerateMathChallenge()
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

func ApplyBreakGlassResistance(targetType friction.TargetType, target string, policy model.FrictionPolicy) {
	info := friction.ResistanceTarget(targetType, target)
	if !info.OK {
		return
	}
	waitSeconds := info.WaitSeconds * 3

	fmt.Printf("\nBREAK GLASS unlock requested for %s\n", info.Name)
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
