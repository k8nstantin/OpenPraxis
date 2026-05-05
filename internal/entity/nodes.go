package entity

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

// NodeEntity is a single SCD-2 version row linking a node to an entity.
// The "current" association for a (node_id, entity_uid) pair is the row
// where valid_to = ''.
type NodeEntity struct {
	RowID        int64  `json:"row_id"`
	NodeID       string `json:"node_id"`
	EntityUID    string `json:"entity_uid"`
	ValidFrom    string `json:"valid_from"`
	ValidTo      string `json:"valid_to"`
	ChangedBy    string `json:"changed_by"`
	ChangeReason string `json:"change_reason"`
	CreatedAt    string `json:"created_at"`
}

func (s *Store) initNodesSchema() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS nodes_entities (
		row_id        INTEGER PRIMARY KEY AUTOINCREMENT,
		node_id       TEXT NOT NULL,
		entity_uid    TEXT NOT NULL,
		valid_from    TEXT NOT NULL,
		valid_to      TEXT NOT NULL DEFAULT '',
		changed_by    TEXT NOT NULL DEFAULT '',
		change_reason TEXT NOT NULL DEFAULT '',
		created_at    TEXT NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("entity: init nodes: create nodes_entities table: %w", err)
	}

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_nodes_entities_node_current
		ON nodes_entities (node_id) WHERE valid_to = ''`)
	if err != nil {
		return fmt.Errorf("entity: init nodes: create node current index: %w", err)
	}

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_nodes_entities_entity_current
		ON nodes_entities (entity_uid) WHERE valid_to = ''`)
	if err != nil {
		return fmt.Errorf("entity: init nodes: create entity current index: %w", err)
	}

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_nodes_entities_history
		ON nodes_entities (node_id, entity_uid, valid_from DESC)`)
	if err != nil {
		return fmt.Errorf("entity: init nodes: create history index: %w", err)
	}

	return nil
}

// AddNodeEntity creates a current nodes_entities row linking nodeID to
// entityUID. Idempotent — if an active row already exists for the pair
// the call is a no-op.
func (s *Store) AddNodeEntity(ctx context.Context, nodeID, entityUID, changedBy, changeReason string) error {
	var existing int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM nodes_entities
		WHERE node_id = ? AND entity_uid = ? AND valid_to = ''`,
		nodeID, entityUID).Scan(&existing)
	if err != nil {
		return fmt.Errorf("entity: add_node_entity: check existing: %w", err)
	}
	if existing > 0 {
		return nil
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.ExecContext(ctx, `INSERT INTO nodes_entities
		(node_id, entity_uid, valid_from, valid_to, changed_by, change_reason, created_at)
		VALUES (?, ?, ?, '', ?, ?, ?)`,
		nodeID, entityUID, now, changedBy, changeReason, now)
	if err != nil {
		return fmt.Errorf("entity: add_node_entity: %w", err)
	}
	return nil
}

// RemoveNodeEntity closes the current nodes_entities row for (nodeID, entityUID)
// by setting valid_to = now. Idempotent — no-op when no active row exists.
func (s *Store) RemoveNodeEntity(ctx context.Context, nodeID, entityUID, changedBy, changeReason string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `UPDATE nodes_entities
		SET valid_to = ?, changed_by = ?, change_reason = ?
		WHERE node_id = ? AND entity_uid = ? AND valid_to = ''`,
		now, changedBy, changeReason, nodeID, entityUID)
	if err != nil {
		return fmt.Errorf("entity: remove_node_entity: %w", err)
	}
	return nil
}

// ListByNode returns all current nodes_entities rows for nodeID.
func (s *Store) ListByNode(nodeID string) ([]*NodeEntity, error) {
	rows, err := s.db.Query(`SELECT row_id, node_id, entity_uid, valid_from, valid_to,
		changed_by, change_reason, created_at
		FROM nodes_entities WHERE node_id = ? AND valid_to = ''
		ORDER BY created_at DESC`, nodeID)
	if err != nil {
		return nil, fmt.Errorf("entity: list_by_node: %w", err)
	}
	defer rows.Close()
	return scanNodeEntities(rows)
}

// ListByEntity returns all current nodes_entities rows for entityUID.
func (s *Store) ListByEntity(entityUID string) ([]*NodeEntity, error) {
	rows, err := s.db.Query(`SELECT row_id, node_id, entity_uid, valid_from, valid_to,
		changed_by, change_reason, created_at
		FROM nodes_entities WHERE entity_uid = ? AND valid_to = ''
		ORDER BY created_at DESC`, entityUID)
	if err != nil {
		return nil, fmt.Errorf("entity: list_by_entity: %w", err)
	}
	defer rows.Close()
	return scanNodeEntities(rows)
}

func scanNodeEntities(rows *sql.Rows) ([]*NodeEntity, error) {
	var results []*NodeEntity
	for rows.Next() {
		var ne NodeEntity
		err := rows.Scan(&ne.RowID, &ne.NodeID, &ne.EntityUID,
			&ne.ValidFrom, &ne.ValidTo, &ne.ChangedBy, &ne.ChangeReason, &ne.CreatedAt)
		if err != nil {
			slog.Warn("entity: scan node_entity row failed", "error", err)
			continue
		}
		results = append(results, &ne)
	}
	return results, rows.Err()
}
