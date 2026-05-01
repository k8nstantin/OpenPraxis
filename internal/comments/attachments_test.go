package comments

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newAttachStore(t *testing.T) (*Store, *AttachmentStore, string) {
	t.Helper()
	db := openTestDB(t)
	if err := InitAttachmentSchema(db); err != nil {
		t.Fatalf("InitAttachmentSchema: %v", err)
	}
	root := filepath.Join(t.TempDir(), "attachments")
	cs := NewStore(db)
	as := NewAttachmentStore(db, root)
	cs.SetAttachments(as)
	return cs, as, root
}

func TestSanitizeFilename(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"hello.png", "hello.png"},
		{"my screenshot.png", "my_screenshot.png"},
		{"../../etc/passwd", "passwd"},
		{"weird*name?.txt", "weird_name_.txt"},
		{"", ""},
	}
	for _, tc := range cases {
		got := SanitizeFilename(tc.in)
		if got != tc.want {
			t.Errorf("SanitizeFilename(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestMimeAllowed_Defaults(t *testing.T) {
	cases := []struct {
		mime string
		want bool
	}{
		{"image/png", true},
		{"image/jpeg", true},
		{"text/plain", true},
		{"application/pdf", true},
		{"application/zip", true},
		{"application/json", true},
		{"application/x-mach-binary", false},
		{"application/octet-stream", false},
		{"", false},
	}
	for _, tc := range cases {
		got := MimeAllowed(tc.mime, nil)
		if got != tc.want {
			t.Errorf("MimeAllowed(%q, default) = %v, want %v", tc.mime, got, tc.want)
		}
	}
}

func TestMimeAllowed_Custom(t *testing.T) {
	if !MimeAllowed("image/png", []string{"image/png"}) {
		t.Error("expected exact match")
	}
	if MimeAllowed("image/jpeg", []string{"image/png"}) {
		t.Error("exact match must not pass other mimes")
	}
}

func TestAttachmentStore_InsertList(t *testing.T) {
	cs, as, root := newAttachStore(t)
	ctx := context.Background()
	c, err := cs.Add(ctx, TargetTask, "task-1", "alice", TypeUserNote, "see attached")
	if err != nil {
		t.Fatalf("add comment: %v", err)
	}

	a, err := as.Insert(ctx, c.ID, "alice", "shot.png", "image/png", []byte("PNGDATA"))
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if a.ID == "" {
		t.Fatal("expected non-empty id")
	}
	if a.SizeBytes != int64(len("PNGDATA")) {
		t.Errorf("size = %d", a.SizeBytes)
	}
	if !strings.HasPrefix(a.StoragePath, root) {
		t.Errorf("storage path %q not under root %q", a.StoragePath, root)
	}
	if _, err := os.Stat(a.StoragePath); err != nil {
		t.Errorf("file missing on disk: %v", err)
	}

	got, err := as.ListByComment(ctx, c.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].ID != a.ID {
		t.Fatalf("list mismatch: %+v", got)
	}
}

func TestAttachmentStore_GetMissing(t *testing.T) {
	_, as, _ := newAttachStore(t)
	if _, err := as.Get(context.Background(), "nope"); !errors.Is(err, ErrAttachmentNotFound) {
		t.Errorf("expected ErrAttachmentNotFound, got %v", err)
	}
}

func TestAttachmentStore_SoftDeleteRemovesFile(t *testing.T) {
	cs, as, _ := newAttachStore(t)
	ctx := context.Background()
	c, _ := cs.Add(ctx, TargetTask, "t1", "a", TypeUserNote, "x")
	a, err := as.Insert(ctx, c.ID, "a", "x.txt", "text/plain", []byte("body"))
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := as.SoftDelete(ctx, a.ID); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	if _, err := os.Stat(a.StoragePath); !os.IsNotExist(err) {
		t.Errorf("file should be gone, stat err = %v", err)
	}
	if _, err := as.Get(ctx, a.ID); !errors.Is(err, ErrAttachmentNotFound) {
		t.Errorf("expected not-found after soft delete, got %v", err)
	}
	// Idempotency: a second delete returns ErrAttachmentNotFound.
	if err := as.SoftDelete(ctx, a.ID); !errors.Is(err, ErrAttachmentNotFound) {
		t.Errorf("expected ErrAttachmentNotFound on double delete, got %v", err)
	}
}

func TestStoreDelete_CascadesAttachments(t *testing.T) {
	cs, as, _ := newAttachStore(t)
	ctx := context.Background()
	c, _ := cs.Add(ctx, TargetManifest, "m1", "a", TypeUserNote, "x")
	a, err := as.Insert(ctx, c.ID, "a", "x.png", "image/png", []byte("P"))
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := cs.Delete(ctx, c.ID); err != nil {
		t.Fatalf("delete comment: %v", err)
	}
	if _, err := os.Stat(a.StoragePath); !os.IsNotExist(err) {
		t.Errorf("attachment file should be gone, stat err = %v", err)
	}
	got, err := as.ListByComment(ctx, c.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected zero live attachments after cascade, got %d", len(got))
	}
}
