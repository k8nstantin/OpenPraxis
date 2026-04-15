package setup

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const (
	ollamaURL    = "http://localhost:11434"
	defaultModel = "nomic-embed-text"
)

// EnsureReady checks all dependencies and installs what's missing.
// Returns nil when everything is ready.
func EnsureReady(model string) error {
	if model == "" {
		model = defaultModel
	}

	fmt.Println("  Checking dependencies...")

	// Step 1: Ollama installed?
	if !ollamaInstalled() {
		fmt.Println("\n  Ollama is required for semantic search (local AI embeddings).")
		fmt.Println("  It runs entirely on your machine — no data leaves your network.")
		if !askPermission("  Install Ollama? [Y/n] ") {
			return fmt.Errorf("Ollama is required. Install manually: https://ollama.com/download")
		}
		if err := installOllama(); err != nil {
			return fmt.Errorf("failed to install Ollama: %w\n\n  Please install manually: https://ollama.com/download", err)
		}
		fmt.Println("  Ollama installed.")
	} else {
		fmt.Println("  Ollama: installed")
	}

	// Step 2: Ollama running?
	if !ollamaRunning() {
		fmt.Println("  Starting Ollama service...")
		if err := startOllama(); err != nil {
			return fmt.Errorf("failed to start Ollama: %w\n\n  Please start manually: ollama serve", err)
		}
		if err := waitForOllama(15 * time.Second); err != nil {
			return fmt.Errorf("Ollama didn't start in time: %w", err)
		}
		fmt.Println("  Ollama: running")
	} else {
		fmt.Println("  Ollama: running")
	}

	// Step 3: Model available?
	if !modelAvailable(model) {
		fmt.Printf("\n  Embedding model '%s' is needed for semantic search.\n", model)
		fmt.Println("  This is a one-time download (~274MB).")
		if !askPermission(fmt.Sprintf("  Download %s? [Y/n] ", model)) {
			return fmt.Errorf("model %s is required for semantic search", model)
		}
		fmt.Printf("  Pulling %s...\n", model)
		if err := pullModel(model); err != nil {
			return fmt.Errorf("failed to pull model %s: %w", model, err)
		}
		fmt.Printf("  Model %s: ready\n", model)
	} else {
		fmt.Printf("  Model %s: ready\n", model)
	}

	fmt.Println("  All dependencies ready.")
	return nil
}

func askPermission(prompt string) bool {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))
	return input == "" || input == "y" || input == "yes"
}

func ollamaInstalled() bool {
	_, err := exec.LookPath("ollama")
	return err == nil
}

func installOllama() error {
	switch runtime.GOOS {
	case "darwin":
		// Try brew first
		if _, err := exec.LookPath("brew"); err == nil {
			cmd := exec.Command("brew", "install", "ollama")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err == nil {
				return nil
			}
		}
		// Fallback to official installer
		fmt.Println("  Downloading from ollama.com...")
		cmd := exec.Command("bash", "-c", "curl -fsSL https://ollama.com/install.sh | sh")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()

	case "linux":
		cmd := exec.Command("bash", "-c", "curl -fsSL https://ollama.com/install.sh | sh")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()

	default:
		return fmt.Errorf("automatic installation not supported on %s — please install from https://ollama.com/download", runtime.GOOS)
	}
}

func ollamaRunning() bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(ollamaURL + "/api/tags")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

func startOllama() error {
	cmd := exec.Command("ollama", "serve")
	cmd.Stdout = nil
	cmd.Stderr = nil
	// Start in background — detach from this process
	if err := cmd.Start(); err != nil {
		return err
	}
	// Don't wait — let it run in background
	go cmd.Wait()
	return nil
}

func waitForOllama(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ollamaRunning() {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timeout after %s", timeout)
}

func modelAvailable(model string) bool {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(ollamaURL + "/api/tags")
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false
	}

	for _, m := range result.Models {
		// Match "nomic-embed-text" against "nomic-embed-text:latest"
		name := strings.Split(m.Name, ":")[0]
		if name == model || m.Name == model {
			return true
		}
	}
	return false
}

func pullModel(model string) error {
	cmd := exec.Command("ollama", "pull", model)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
