package daemon

import "os/exec"

func redirectAdd() {
	c := exec.Command("iptables", "-t", "nat", "-C", "OUTPUT", "-p", "tcp", "--dport", "443",
		"-j", "REDIRECT", "--to-port", "8443", "-m", "comment", "--comment", "bakchodi-sni")
	if c.Run() == nil {
		return
	}
	exec.Command("iptables", "-t", "nat", "-A", "OUTPUT", "-p", "tcp", "--dport", "443",
		"-j", "REDIRECT", "--to-port", "8443", "-m", "comment", "--comment", "bakchodi-sni").Run()
	exec.Command("ip6tables", "-t", "nat", "-A", "OUTPUT", "-p", "tcp", "--dport", "443",
		"-j", "REDIRECT", "--to-port", "8443", "-m", "comment", "--comment", "bakchodi-sni").Run()
	exec.Command("iptables", "-A", "OUTPUT", "-p", "udp", "--dport", "443",
		"-j", "REJECT", "--reject-with", "icmp-port-unreachable",
		"-m", "comment", "--comment", "bakchodi-quic").Run()
	exec.Command("ip6tables", "-A", "OUTPUT", "-p", "udp", "--dport", "443",
		"-j", "REJECT", "--reject-with", "icmp6-port-unreachable",
		"-m", "comment", "--comment", "bakchodi-quic").Run()
}

func redirectDel() {
	exec.Command("iptables", "-t", "nat", "-D", "OUTPUT", "-p", "tcp", "--dport", "443",
		"-j", "REDIRECT", "--to-port", "8443", "-m", "comment", "--comment", "bakchodi-sni").Run()
	exec.Command("ip6tables", "-t", "nat", "-D", "OUTPUT", "-p", "tcp", "--dport", "443",
		"-j", "REDIRECT", "--to-port", "8443", "-m", "comment", "--comment", "bakchodi-sni").Run()
	exec.Command("iptables", "-D", "OUTPUT", "-p", "udp", "--dport", "443",
		"-j", "REJECT", "--reject-with", "icmp-port-unreachable",
		"-m", "comment", "--comment", "bakchodi-quic").Run()
	exec.Command("ip6tables", "-D", "OUTPUT", "-p", "udp", "--dport", "443",
		"-j", "REJECT", "--reject-with", "icmp6-port-unreachable",
		"-m", "comment", "--comment", "bakchodi-quic").Run()
}
