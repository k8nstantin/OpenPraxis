package manifest

import (
	"fmt"
	"time"
)

// Link represents a many-to-many relationship between an idea and a manifest.
type Link struct {
	ID           int       `json:"id"`
	IdeaID       string    `json:"idea_id"`
	IdeaMarker   string    `json:"idea_marker"`
	ManifestID   string    `json:"manifest_id"`
	ManifestMark string    `json:"manifest_marker"`
	CreatedAt    time.Time `json:"created_at"`
}

// IdeaRef is a lightweight idea reference shown on manifest detail.
type IdeaRef struct {
	ID     string `json:"id"`
	Marker string `json:"marker"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

// ManifestRef is a lightweight manifest reference shown on idea detail.
type ManifestRef struct {
	ID     string `json:"id"`
	Marker string `json:"marker"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

// InitLinks creates the join table.
func (s *Store) InitLinks() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS idea_manifest_links (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		idea_id TEXT NOT NULL,
		manifest_id TEXT NOT NULL,
		created_at TEXT NOT NULL,
		UNIQUE(idea_id, manifest_id)
	)`)
	if err != nil {
		return fmt.Errorf("create links table: %w", err)
	}

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_links_idea ON idea_manifest_links(idea_id)`)
	if err != nil {
		return fmt.Errorf("create links idea index: %w", err)
	}

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_links_manifest ON idea_manifest_links(manifest_id)`)
	return err
}

// LinkIdeaToManifest creates a link between an idea and manifest.
func (s *Store) LinkIdeaToManifest(ideaID, manifestID string) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO idea_manifest_links (idea_id, manifest_id, created_at) VALUES (?, ?, ?)`,
		ideaID, manifestID, time.Now().UTC().Format(time.RFC3339))
	return err
}

// UnlinkIdeaFromManifest removes a link.
func (s *Store) UnlinkIdeaFromManifest(ideaID, manifestID string) error {
	_, err := s.db.Exec(`DELETE FROM idea_manifest_links WHERE idea_id = ? AND manifest_id = ?`, ideaID, manifestID)
	return err
}

// ManifestsForIdea returns all manifests linked to an idea.
func (s *Store) ManifestsForIdea(ideaID string) ([]ManifestRef, error) {
	rows, err := s.db.Query(`SELECT m.id, m.title, m.status FROM manifests m
		JOIN idea_manifest_links l ON m.id = l.manifest_id
		WHERE l.idea_id = ? OR l.idea_id LIKE ?
		ORDER BY m.updated_at DESC`, ideaID, ideaID+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []ManifestRef
	for rows.Next() {
		var r ManifestRef
		if err := rows.Scan(&r.ID, &r.Title, &r.Status); err != nil {
			return nil, err
		}
		if len(r.ID) >= 12 {
			r.Marker = r.ID[:12]
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// IdeasForManifest returns all ideas linked to a manifest.
func (s *Store) IdeasForManifest(manifestID string) ([]IdeaRef, error) {
	rows, err := s.db.Query(`SELECT i.id, i.title, i.status FROM ideas i
		JOIN idea_manifest_links l ON i.id = l.idea_id
		WHERE l.manifest_id = ? OR l.manifest_id LIKE ?
		ORDER BY i.updated_at DESC`, manifestID, manifestID+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []IdeaRef
	for rows.Next() {
		var r IdeaRef
		if err := rows.Scan(&r.ID, &r.Title, &r.Status); err != nil {
			return nil, err
		}
		if len(r.ID) >= 12 {
			r.Marker = r.ID[:12]
		}
		results = append(results, r)
	}
	return results, rows.Err()
}
