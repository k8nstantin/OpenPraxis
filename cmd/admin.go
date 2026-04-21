package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/k8nstantin/OpenPraxis/internal/comments"
	"github.com/k8nstantin/OpenPraxis/internal/config"
	"github.com/k8nstantin/OpenPraxis/internal/node"

	"github.com/spf13/cobra"
)

var adminCmd = &cobra.Command{
	Use:   "admin",
	Short: "Operator-only maintenance commands",
}

var migrateCommentOrphansApply bool

var migrateCommentOrphansCmd = &cobra.Command{
	Use:   "migrate-comment-orphans",
	Short: "Rewrite short-marker comment target_ids to full UUIDs",
	Long: `Scans the comments table for rows whose target_id is a short
marker (length < 36). For each row, looks up the canonical full UUID via
the product/manifest/task stores (which accept marker OR full id). Rows
whose target cannot be resolved are logged and left alone — nothing is
deleted. Dry-run by default; pass --apply to actually write.

Motivation: before PRs #136 (MCP comment_add) and #139 (HTTP comment
handler) landed, both paths stored target_id verbatim, so comments
posted with short markers became invisible to the dashboard (which
queries by full UUID). This command sweeps the legacy rows.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})))

		cfg, err := config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		n, err := node.New(cfg)
		if err != nil {
			return fmt.Errorf("init node: %w", err)
		}
		defer n.Close()

		resolver := &nodeTargetResolver{n: n}
		report, err := comments.SweepOrphans(context.Background(), n.Index.DB(), resolver, !migrateCommentOrphansApply)
		if err != nil {
			return fmt.Errorf("sweep: %w", err)
		}

		mode := "dry-run"
		if migrateCommentOrphansApply {
			mode = "apply"
		}
		fmt.Printf("orphan comment sweep (%s)\n", mode)
		fmt.Printf("  scanned:      %d\n", report.Scanned)
		fmt.Printf("  migrated:     %d\n", report.Migrated)
		fmt.Printf("  unresolvable: %d\n", report.Unresolvable)
		if len(report.ByTargetType) > 0 {
			fmt.Printf("  by target_type:\n")
			for _, tt := range []string{"product", "manifest", "task"} {
				if c := report.ByTargetType[tt]; c > 0 {
					fmt.Printf("    %-9s %d\n", tt+":", c)
				}
			}
		}
		if !migrateCommentOrphansApply && report.Migrated > 0 {
			fmt.Printf("\n(dry-run — re-run with --apply to write changes)\n")
		}
		return nil
	},
}

// nodeTargetResolver adapts node.Node to comments.TargetResolver by
// dispatching to the product/manifest/task store Get methods. Each of
// those stores already accepts a marker prefix OR a full UUID via the
// `id = ? OR id LIKE ?` query form, so short markers resolve naturally.
type nodeTargetResolver struct{ n *node.Node }

func (r *nodeTargetResolver) Resolve(ctx context.Context, t comments.TargetType, id string) (string, error) {
	switch t {
	case comments.TargetTask:
		tk, err := r.n.Tasks.Get(id)
		if err != nil || tk == nil {
			return "", nil
		}
		return tk.ID, nil
	case comments.TargetManifest:
		m, err := r.n.Manifests.Get(id)
		if err != nil || m == nil {
			return "", nil
		}
		return m.ID, nil
	case comments.TargetProduct:
		p, err := r.n.Products.Get(id)
		if err != nil || p == nil {
			return "", nil
		}
		return p.ID, nil
	}
	return "", nil
}

func init() {
	migrateCommentOrphansCmd.Flags().BoolVar(&migrateCommentOrphansApply, "apply", false, "actually write UPDATEs (default: dry-run)")
	adminCmd.AddCommand(migrateCommentOrphansCmd)
	rootCmd.AddCommand(adminCmd)
}
