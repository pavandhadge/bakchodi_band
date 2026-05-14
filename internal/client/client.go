package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"

	"github.com/pavandhadge/dopamine_blocker/internal/platform"
)

type Client struct {
	Config platform.Config
}

func New(cfg platform.Config) Client {
	return Client{Config: cfg}
}

func (c Client) Block(targetType, target string) error {
	return c.send("/"+targetType, map[string]string{"target": target})
}

func (c Client) Unlock(targetType, target string) error {
	ApplyResistance(targetType, target)
	return c.send("/"+targetType, map[string]string{"target": target})
}

func (c Client) send(endpoint string, payload map[string]string) error {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	httpClient := http.Client{}
	url := "http://localhost" + endpoint
	if c.Config.SocketNetwork == "unix" {
		httpClient.Transport = &http.Transport{Dial: func(network, addr string) (net.Conn, error) {
			return net.Dial("unix", c.Config.SocketAddress)
		}}
		url = "http://unix" + endpoint
	}

	resp, err := httpClient.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to connect to daemon: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s", bytes.TrimSpace(body))
	}
	_, err = os.Stdout.Write(body)
	return err
}
