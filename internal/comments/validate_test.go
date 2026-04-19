package comments

import (
	"errors"
	"testing"
)

func TestValidateAdd(t *testing.T) {
	cases := []struct {
		name   string
		target TargetType
		tid    string
		author string
		cType  CommentType
		body   string
		want   error
	}{
		{"ok product", TargetProduct, "p1", "alice", TypeUserNote, "hello", nil},
		{"ok manifest", TargetManifest, "m1", "bob", TypeDecision, "decided", nil},
		{"ok task", TargetTask, "t1", "carol", TypeAgentNote, "noted", nil},

		{"unknown target type", TargetType("peer"), "id", "a", TypeUserNote, "body", ErrUnknownTargetType},
		{"empty target type", TargetType(""), "id", "a", TypeUserNote, "body", ErrUnknownTargetType},

		{"empty target id", TargetProduct, "", "a", TypeUserNote, "body", ErrEmptyTargetID},

		{"empty author", TargetProduct, "id", "", TypeUserNote, "body", ErrEmptyAuthor},

		{"unknown comment type", TargetProduct, "id", "a", CommentType("ranty"), "body", ErrUnknownCommentType},
		{"empty comment type", TargetProduct, "id", "a", CommentType(""), "body", ErrUnknownCommentType},

		{"empty body", TargetProduct, "id", "a", TypeUserNote, "", ErrEmptyBody},
		{"whitespace-only body", TargetProduct, "id", "a", TypeUserNote, "   \t\n ", ErrEmptyBody},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ValidateAdd(tc.target, tc.tid, tc.author, tc.cType, tc.body)
			if tc.want == nil {
				if got != nil {
					t.Fatalf("expected nil, got %v", got)
				}
				return
			}
			if !errors.Is(got, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestValidateEdit(t *testing.T) {
	if err := ValidateEdit("new body"); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if err := ValidateEdit(""); !errors.Is(err, ErrEmptyBody) {
		t.Fatalf("expected ErrEmptyBody, got %v", err)
	}
	if err := ValidateEdit("   \n\t "); !errors.Is(err, ErrEmptyBody) {
		t.Fatalf("expected ErrEmptyBody for whitespace-only, got %v", err)
	}
}

func TestRegistry(t *testing.T) {
	reg := Registry()
	all := AllCommentTypes()

	if len(reg) != 6 {
		t.Fatalf("Registry len = %d, want 6", len(reg))
	}
	if len(reg) != len(all) {
		t.Fatalf("Registry (%d) and AllCommentTypes (%d) length mismatch", len(reg), len(all))
	}

	for i, info := range reg {
		if info.Type != all[i] {
			t.Errorf("Registry[%d].Type = %q, want %q (taxonomy order)", i, info.Type, all[i])
		}
		if info.Label == "" {
			t.Errorf("Registry[%d] (%s) has empty Label", i, info.Type)
		}
		if info.Description == "" {
			t.Errorf("Registry[%d] (%s) has empty Description", i, info.Type)
		}
		if !IsValidCommentType(string(info.Type)) {
			t.Errorf("Registry[%d] type %q not recognised by IsValidCommentType", i, info.Type)
		}
	}
}

func TestAllCommentTypesRoundTrip(t *testing.T) {
	for _, ct := range AllCommentTypes() {
		if !IsValidCommentType(string(ct)) {
			t.Errorf("AllCommentTypes contains %q which fails IsValidCommentType", ct)
		}
	}
}
