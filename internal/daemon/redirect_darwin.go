package daemon

import (
	"fmt"
	"os/exec"
)

func redirectAdd() {
	c := exec.Command("pfctl", "-a", "bakchodi-band/quic", "-F", "rules")
	c.Run()

	rdr := "block out proto udp to any port 443\n"
	cmd := exec.Command("pfctl", "-a", "bakchodi-band/quic", "-f", "-")
	stdin, _ := cmd.StdinPipe()
	go func() {
		defer stdin.Close()
		fmt.Fprint(stdin, rdr)
	}()
	if err := cmd.Run(); err != nil {
		exec.Command("pfctl", "-e").Run()
		cmd2 := exec.Command("pfctl", "-a", "bakchodi-band/quic", "-f", "-")
		stdin2, _ := cmd2.StdinPipe()
		go func() {
			defer stdin2.Close()
			fmt.Fprint(stdin2, rdr)
		}()
		cmd2.Run()
	}
}

func redirectDel() {
	exec.Command("pfctl", "-a", "bakchodi-band/quic", "-F", "rules").Run()
}
