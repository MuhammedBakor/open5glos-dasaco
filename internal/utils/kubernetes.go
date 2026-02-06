package utils

import (
	"os/exec"
	"strings"
)

func GetMinikubeIP() (string, error) {
	out, err := exec.Command("minikube", "ip").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
