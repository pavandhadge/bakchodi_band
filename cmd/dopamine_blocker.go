package main

import (
	"fmt"
	"os"
)

func main() {
	// 1. Check if the user provided any command at all
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	// 2. The first argument determines the "Personality" of the app
	command := os.Args[1]

	switch command {
	case "daemon":
		// PATH A: Run as the background server
		runDaemon()

	case "block":
		// PATH B: Run as the client to tell the daemon to block
		runClientBlock()

	case "unlock":
		// PATH C: Run as the client to tell the daemon to unlock
		runClientUnlock()

	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Dopamine Locker")
	fmt.Println("Usage:")
	fmt.Println("  dopelock daemon                      - Start the background worker (Requires Root)")
	fmt.Println("  dopelock block --url <url>           - Lock a specific URL")
	fmt.Println("  dopelock block --group <name>        - Lock a specific group")
	fmt.Println("  dopelock block --all                 - Lock all sites")
	fmt.Println("  dopelock unlock --url <url>          - Unlock a specific URL")
	fmt.Println("  dopelock unlock --group <name>       - Unlock a specific group")
	fmt.Println("  dopelock unlock --all                - Unlock all sites")
	fmt.Println("")
	fmt.Println("Short forms:")
	fmt.Println("  -u, --url   |   -g, --group   |   -a, --all")
}