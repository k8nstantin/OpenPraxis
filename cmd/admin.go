package cmd

import (
	"database/sql"
	"fmt"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
	"github.com/spf13/cobra"

	"github.com/k8nstantin/OpenPraxis/internal/comments"
	"github.com/k8nstantin/OpenPraxis/internal/config"
)

var adminApply bool

var adminCmd = &cobra.Command{
	Use:   "admin",
	Short: "One-shot administrative commands",
	Long:  "Administrative utilities for maintaining an OpenPraxis data directory.",
}

var adminBackfillDescriptionRevisionsCmd = &cobra.Command{
	Use:   "backfill-description-revisions",
	Short: "Seed v1 description_revision comments for existing entities",
	Long: `Seeds one description_revision comment for every existing product,
manifest, and task whose description/content text is non-empty and that
does not already carry a description_revision row.

Default is a dry run that reports counts; pass --apply to write rows.
Safe to re-run — the INSERT is guarded so a second invocation writes zero.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		dbPath := filepath.Join(cfg.Storage.DataDir, "memories.db")
		db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
		if err != nil {
			return fmt.Errorf("open %s: %w", dbPath, err)
		}
		defer db.Close()
		if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
			return fmt.Errorf("set WAL: %w", err)
		}
		if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
			return fmt.Errorf("set busy_timeout: %w", err)
		}

		if err := comments.InitSchema(db); err != nil {
			return fmt.Errorf("init comments schema: %w", err)
		}

		rep, err := comments.BackfillDescriptionRevisions(cmd.Context(), db, adminApply)
		if err != nil {
			return err
		}

		label := "would seed"
		if adminApply {
			label = "seeded"
		}
		fmt.Printf("description_revision backfill (%s)\n", modeLabel(adminApply))
		fmt.Printf("  products:  %s %d, skipped %d (already had a revision)\n", label, rep.ProductsSeeded, rep.ProductsSkipped)
		fmt.Printf("  manifests: %s %d, skipped %d\n", label, rep.ManifestsSeeded, rep.ManifestsSkipped)
		fmt.Printf("  tasks:     %s %d, skipped %d\n", label, rep.TasksSeeded, rep.TasksSkipped)
		fmt.Printf("  total:     %s %d\n", label, rep.Total())
		if !adminApply {
			fmt.Println("\n(dry run — re-run with --apply to write rows)")
		}
		return nil
	},
}

func modeLabel(apply bool) string {
	if apply {
		return "apply"
	}
	return "dry-run"
}

func init() {
	rootCmd.AddCommand(adminCmd)
	adminCmd.AddCommand(adminBackfillDescriptionRevisionsCmd)
	adminBackfillDescriptionRevisionsCmd.Flags().BoolVar(&adminApply, "apply", false, "Actually write rows (default is dry-run)")
}

