package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/pavandhadge/dopamine_blocker/internal/client"
	"github.com/pavandhadge/dopamine_blocker/internal/daemon"
	"github.com/pavandhadge/dopamine_blocker/internal/platform"
	"github.com/pavandhadge/dopamine_blocker/internal/tui"
)

func main() {
	cfg := platform.DefaultConfig()
	if len(os.Args) < 2 {
		if err := tui.Run(cfg); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
		return
	}

	switch os.Args[1] {
	case "tui":
		if err := tui.Run(cfg); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
	case "band", "start", "daemon":
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
	case "allow", "unlock":
		target, targetType := parseTargetFlags("unlock", os.Args[2:])
		if err := client.New(cfg).Unlock(targetType, target); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
	case "panic", "break-glass":
		target, targetType := parseTargetFlags("unlock", os.Args[2:])
		if err := client.New(cfg).BreakGlass(targetType, target); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
	case "plan", "commit":
		hours, reason := parseCommitFlags(os.Args[2:])
		if err := client.New(cfg).Commit(hours, reason); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
	case "add-group", "group":
		name, urls, file := parseGroupFlags(os.Args[2:])
		if err := client.New(cfg).AddGroup(name, urls, file); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
	case "import-groups":
		file, merge := parseImportGroupsFlags(os.Args[2:])
		if err := client.New(cfg).ImportGroups(file, merge); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
	case "setup", "import-config":
		file := parseFileFlag("import-config", os.Args[2:])
		if err := client.New(cfg).ImportConfig(file); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
	case "sample-config", "default-config":
		file := parseOutputFlag(os.Args[2:])
		if err := client.WriteDefaultConfig(file); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
		fmt.Println("Wrote default config to", file)
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

type stringList []string

func (s *stringList) String() string {
	return strings.Join(*s, ",")
}

func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func parseGroupFlags(args []string) (string, []string, string) {
	fs := flag.NewFlagSet("group", flag.ExitOnError)
	name := fs.String("name", "", "group name")
	file := fs.String("file", "", "plain text URL file")
	var urls stringList
	fs.Var(&urls, "url", "URL/domain to add; repeat for multiple")
	fs.StringVar(name, "n", "", "group name")
	fs.StringVar(file, "f", "", "plain text URL file")
	fs.Usage = printUsage
	_ = fs.Parse(args)
	return *name, urls, *file
}

func parseImportGroupsFlags(args []string) (string, bool) {
	fs := flag.NewFlagSet("import-groups", flag.ExitOnError)
	file := fs.String("file", "", "JSON groups file")
	merge := fs.Bool("merge", true, "merge into existing groups")
	fs.StringVar(file, "f", "", "JSON groups file")
	fs.Usage = printUsage
	_ = fs.Parse(args)
	if *file == "" {
		fmt.Fprintln(os.Stderr, "Error: --file is required")
		os.Exit(1)
	}
	return *file, *merge
}

func parseFileFlag(command string, args []string) string {
	fs := flag.NewFlagSet(command, flag.ExitOnError)
	file := fs.String("file", "", "file path")
	fs.StringVar(file, "f", "", "file path")
	fs.Usage = printUsage
	_ = fs.Parse(args)
	if *file == "" {
		fmt.Fprintln(os.Stderr, "Error: --file is required")
		os.Exit(1)
	}
	return *file
}

func parseOutputFlag(args []string) string {
	fs := flag.NewFlagSet("default-config", flag.ExitOnError)
	out := fs.String("out", "bakchodi.config.json", "output file path")
	fs.StringVar(out, "o", "bakchodi.config.json", "output file path")
	fs.Usage = printUsage
	_ = fs.Parse(args)
	return *out
}

func parseCommitFlags(args []string) (int, string) {
	fs := flag.NewFlagSet("commit", flag.ExitOnError)
	hours := fs.Int("hours", 24, "commitment duration in hours")
	reason := fs.String("reason", "", "commitment reason")
	fs.IntVar(hours, "h", 24, "commitment duration in hours")
	fs.StringVar(reason, "r", "", "commitment reason")
	fs.Usage = printUsage
	_ = fs.Parse(args)
	if *hours <= 0 {
		fmt.Fprintln(os.Stderr, "Error: --hours must be greater than zero")
		os.Exit(1)
	}
	return *hours, *reason
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
	fmt.Println("bakchodi_band")
	fmt.Println("Usage:")
	fmt.Println("  bakchodi tui                         - Launch the interactive TUI")
	fmt.Println("  bakchodi band                        - Start the background blocker as admin/root")
	fmt.Println("  bakchodi block --url youtube.com     - Block one site")
	fmt.Println("  bakchodi block --group social        - Block a saved group")
	fmt.Println("  bakchodi block --all                 - Block every saved group")
	fmt.Println("  bakchodi allow --url youtube.com     - Temporarily allow one site")
	fmt.Println("  bakchodi allow --group social        - Temporarily allow one group")
	fmt.Println("  bakchodi allow --all                 - Temporarily allow all blocked sites")
	fmt.Println("  bakchodi panic --url youtube.com     - Emergency 5-minute allow, heavily logged")
	fmt.Println("  bakchodi plan --hours 24             - Prevent normal allows for a fixed period")
	fmt.Println("  bakchodi add-group --name social --url x.com --url reddit.com")
	fmt.Println("  bakchodi add-group --name social --file social.txt")
	fmt.Println("  bakchodi setup --file bakchodi.config.json")
	fmt.Println("  bakchodi sample-config --out bakchodi.config.json")
	fmt.Println("")
	fmt.Println("Short forms:")
	fmt.Println("  -u, --url   |   -g, --group   |   -a, --all")
	fmt.Println("")
	fmt.Println("Old command names still work: start, daemon, unlock, break-glass, commit, group, default-config.")
}
