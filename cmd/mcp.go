package cmd

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"openpraxis/internal/config"
	mcpserver "openpraxis/internal/mcp"
	"openpraxis/internal/node"

	mcplib "github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run as stdio MCP server (for Claude Code to spawn)",
	Long:  "Runs the MCP protocol over stdin/stdout. Claude Code launches this as a subprocess.",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Suppress all output to stdout — MCP protocol uses it
		// Errors go to stderr only
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})))

		cfg, err := config.Load(cfgFile)
		if err != nil {
			slog.Error("config error", "error", err)
			return err
		}

		n, err := node.New(cfg)
		if err != nil {
			slog.Error("init error", "error", err)
			return err
		}
		defer n.Close()

		// Create MCP server with all tools
		mcp := mcpserver.NewServer(n, n.Index.DB())

		// Wrap in stdio transport
		stdio := mcplib.NewStdioServer(mcp.MCPServer())

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Handle shutdown
		go func() {
			done := make(chan os.Signal, 1)
			signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
			<-done
			cancel()
		}()

		// Run stdio — blocks until stdin closes or context cancelled
		if err := stdio.Listen(ctx, os.Stdin, os.Stdout); err != nil {
			slog.Error("stdio error", "error", err)
			return err
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}
