package vpn

import (
	"errors"
	"os/exec"
	"strings"
	"time"
)

var (
	ErrVPNNotConnected = errors.New("VPN not connected")
	ErrVPNConnectFail  = errors.New("failed to connect VPN")
)

type Config struct {
	ActivationCode string
	AutoConnect    bool
	Region         string
}

type ExpressVPN struct {
	cfg *Config
}

func NewExpressVPN(cfg *Config) *ExpressVPN {
	return &ExpressVPN{cfg: cfg}
}

func (v *ExpressVPN) IsConnected() bool {
	out, err := exec.Command("expressvpnctl", "status").Output()
	if err != nil {
		return false
	}
	status := strings.ToLower(string(out))
	return strings.Contains(status, "connected") && !strings.Contains(status, "disconnected")
}

func (v *ExpressVPN) Connect() error {
	if v.IsConnected() {
		return nil
	}

	if !v.cfg.AutoConnect {
		return ErrVPNNotConnected
	}

	region := v.cfg.Region
	if region == "" {
		region = "smart"
	}

	cmd := exec.Command("expressvpnctl", "connect", region)
	if err := cmd.Run(); err != nil {
		return ErrVPNConnectFail
	}

	for i := 0; i < 30; i++ {
		time.Sleep(time.Second)
		if v.IsConnected() {
			return nil
		}
	}

	return ErrVPNConnectFail
}

func (v *ExpressVPN) EnsureConnected() error {
	if v.IsConnected() {
		return nil
	}
	return v.Connect()
}

func (v *ExpressVPN) Disconnect() error {
	return exec.Command("expressvpnctl", "disconnect").Run()
}

func (v *ExpressVPN) GetStatus() (string, error) {
	out, err := exec.Command("expressvpnctl", "status").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
