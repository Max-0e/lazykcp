package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Max-0e/lazykcp/internal/kcp"
	"github.com/Max-0e/lazykcp/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	kubeconfig := flag.String("kubeconfig", "", "Path to a kubeconfig file (defaults to the standard kubeconfig loading rules)")
	insecureSkipTLSVerify := flag.Bool("insecure-skip-tls-verify", false, "Skip TLS certificate verification (unsafe; for local development only)")
	flag.Parse()

	client, err := kcp.NewClient(*kubeconfig, *insecureSkipTLSVerify)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	model := ui.NewModel(client)
	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, runErr := program.Run()
	cleanupErr := model.Close()
	if runErr != nil {
		fmt.Fprintln(os.Stderr, runErr)
	}
	if cleanupErr != nil {
		fmt.Fprintln(os.Stderr, cleanupErr)
	}
	if runErr != nil || cleanupErr != nil {
		os.Exit(1)
	}
}
