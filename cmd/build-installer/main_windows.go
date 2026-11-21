package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	if err := build(); err != nil {
		log.Fatal(err)
	}
}

func build() error {
	payload := filepath.Join("cmd", "installer", "payload", "agent.exe")
	if _, err := os.Stat("go.mod"); err != nil {
		return fmt.Errorf("ejecuta el compilador desde la raíz: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(payload), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll("dist", 0755); err != nil {
		return err
	}
	compile := func(args ...string) error {
		cmd := exec.Command("go", args...)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		return cmd.Run()
	}
	if err := compile("build", "-trimpath", "-o", payload, "./cmd/agent"); err != nil {
		return err
	}
	defer os.Remove(payload)
	if err := compile("vet", "-tags", "installer", "./cmd/installer"); err != nil {
		return err
	}
	output := filepath.Join("dist", "FoodTruckPrinterSetup.exe")
	if err := compile("build", "-trimpath", "-tags", "installer", "-ldflags", "-H=windowsgui", "-o", output, "./cmd/installer"); err != nil {
		return err
	}
	fmt.Println(output)
	return nil
}
