package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

func runClientBlock() {
	target, targetType := parseBlockArgs()
	sendToDaemon("/"+targetType, map[string]string{"target": target})
}

func runClientUnlock() {
	target, targetType := parseUnlockArgs()
	applyResistance(targetType, target)
	sendToDaemon("/"+targetType, map[string]string{"target": target})
}

func applyResistance(targetType, target string) {
	var waitSeconds int
	var targetName string

	switch targetType {
	case "unlock":
		waitSeconds = 60
		targetName = "all sites"
	case "unlock-group":
		waitSeconds = 60
		targetName = "group: " + target
	case "unlock-url":
		waitSeconds = 30
		targetName = "URL: " + target
	default:
		return
	}

	fmt.Printf("\n⚠️  You're about to unlock %s\n", targetName)
	fmt.Printf("⏳ Wait %d seconds to confirm you really want this...\n\n", waitSeconds)

	for i := waitSeconds; i > 0; i-- {
		fmt.Printf("\r⏱️  Time remaining: %2d seconds ", i)
		time.Sleep(1 * time.Second)
	}
	fmt.Println("\n\n✅ Time's up!")

	for attempts := 0; attempts < 3; attempts++ {
		fmt.Println("\n📝 Solve this simple math problem to confirm:")
		a := rand.Intn(10) + 1
		b := rand.Intn(10) + 1
		answer := a + b

		fmt.Printf("   %d + %d = ? ", a, b)

		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		userAnswer, err := strconv.Atoi(input)
		if err != nil {
			fmt.Println("❌ Invalid number. Try again.")
			continue
		}

		if userAnswer == answer {
			fmt.Println("✅ Correct! Proceeding with unlock...")
			return
		}

		fmt.Printf("❌ Wrong answer. The correct answer was %d.\n", answer)
		if attempts < 2 {
			fmt.Printf("📌 Attempt %d/3. Try again.\n", attempts+1)
		}
	}

	fmt.Println("\n❌ Too many failed attempts. Unlock cancelled.")
	os.Exit(0)
}

func parseBlockArgs() (string, string) {
	if len(os.Args) < 3 {
		printUsage()
		os.Exit(1)
	}

	flag := os.Args[2]
	switch flag {
	case "--url", "-u":
		if len(os.Args) < 4 {
			fmt.Println("Error: URL required")
			os.Exit(1)
		}
		return os.Args[3], "lock-url"
	case "--group", "-g":
		if len(os.Args) < 4 {
			fmt.Println("Error: Group name required")
			os.Exit(1)
		}
		return os.Args[3], "lock-group"
	case "--all", "-a":
		return "", "lock"
	default:
		fmt.Println("Unknown flag:", flag)
		printUsage()
		os.Exit(1)
		return "", ""
	}
}

func parseUnlockArgs() (string, string) {
	if len(os.Args) < 3 {
		printUsage()
		os.Exit(1)
	}

	flag := os.Args[2]
	switch flag {
	case "--url", "-u":
		if len(os.Args) < 4 {
			fmt.Println("Error: URL required")
			os.Exit(1)
		}
		return os.Args[3], "unlock-url"
	case "--group", "-g":
		if len(os.Args) < 4 {
			fmt.Println("Error: Group name required")
			os.Exit(1)
		}
		return os.Args[3], "unlock-group"
	case "--all", "-a":
		return "", "unlock"
	default:
		fmt.Println("Unknown flag:", flag)
		printUsage()
		os.Exit(1)
		return "", ""
	}
}

func sendToDaemon(endpoint string, payload map[string]string) {
	var body io.Reader
	if payload != nil {
		jsonData, _ := json.Marshal(payload)
		body = bytes.NewBuffer(jsonData)
	}

	var client http.Client
	if SOCKET_NETWORK == "unix" {
		client = http.Client{Transport: &http.Transport{
			Dial: dialUnix,
		}}
	} else {
		client = http.Client{}
	}

	url := "http://localhost" + endpoint
	if SOCKET_NETWORK == "unix" {
		url = "http://unix" + endpoint
	}

	resp, err := client.Post(url, "application/json", body)
	if err != nil {
		fmt.Println("Failed to connect to daemon:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		fmt.Printf("Error: %s\n", respBody)
		os.Exit(1)
	}
	fmt.Print(string(respBody))
}

func dialUnix(network, addr string) (net.Conn, error) {
	return net.Dial("unix", SOCKET_PATH)
}