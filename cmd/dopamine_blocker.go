package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/pavandhadge/dopamine_blocker/internal/client"
	"github.com/pavandhadge/dopamine_blocker/internal/daemon"
	"github.com/pavandhadge/dopamine_blocker/internal/platform"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cfg := platform.DefaultConfig()
	switch os.Args[1] {
	case "daemon":
		if err := daemon.New(cfg).Run(); err != nil {
			fmt.Fprintln(os.Stderr, "FATAL:", err)
			os.Exit(1)
		}
	case "block":
		target, targetType := parseTargetFlags("block", os.Args[2:])
		if err := client.New(cfg).Block(targetType, target); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
	case "unlock":
		target, targetType := parseTargetFlags("unlock", os.Args[2:])
		if err := client.New(cfg).Unlock(targetType, target); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func parseTargetFlags(command string, args []string) (string, string) {
	fs := flag.NewFlagSet(command, flag.ExitOnError)
	url := fs.String("url", "", "target URL")
	group := fs.String("group", "", "target group")
	all := fs.Bool("all", false, "target all sites")
	fs.StringVar(url, "u", "", "target URL")
	fs.StringVar(group, "g", "", "target group")
	fs.BoolVar(all, "a", false, "target all sites")
	fs.Usage = printUsage
	_ = fs.Parse(args)

	selected := 0
	if *url != "" {
		selected++
	}
	if *group != "" {
		selected++
	}
	if *all {
		selected++
	}
	if selected != 1 {
		fmt.Fprintln(os.Stderr, "Error: choose exactly one of --url, --group, or --all")
		printUsage()
		os.Exit(1)
	}

	prefix := "lock"
	if command == "unlock" {
		prefix = "unlock"
	}
	switch {
	case *url != "":
		return *url, prefix + "-url"
	case *group != "":
		return *group, prefix + "-group"
	default:
		return "", prefix
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
