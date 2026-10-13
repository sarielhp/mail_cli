package email

import (
	"testing"
	"time"
)

func TestBuildThreadTree_CyclePrevention(t *testing.T) {
	now := time.Now()
	// Mutual circular reply: A replies to B, B replies to A
	emails := []Email{
		{
			MessageID: "<msgA@example.com>",
			InReplyTo: "<msgB@example.com>",
			Subject:   "Circular Thread",
			EmailDate: now.Add(-2 * time.Minute),
		},
		{
			MessageID: "<msgB@example.com>",
			InReplyTo: "<msgA@example.com>",
			Subject:   "Circular Thread",
			EmailDate: now.Add(-1 * time.Minute),
		},
	}

	roots := BuildThreadTree(emails)
	if len(roots) == 0 {
		t.Fatal("expected at least one root node")
	}

	// Traversal functions should terminate cleanly without stack overflow
	count := CountThreadReplies(roots[0])
	if count < 0 {
		t.Errorf("unexpected count: %d", count)
	}

	newest := GetThreadNewestDate(roots[0])
	if newest.IsZero() {
		t.Error("expected non-zero newest date")
	}
}

func TestBuildThreadTree_ThreeNodeCycle(t *testing.T) {
	now := time.Now()
	// 3-node cycle: A -> B -> C -> A
	emails := []Email{
		{
			MessageID: "<msg1@example.com>",
			InReplyTo: "<msg3@example.com>",
			Subject:   "Three Node Loop",
			EmailDate: now.Add(-3 * time.Minute),
		},
		{
			MessageID: "<msg2@example.com>",
			InReplyTo: "<msg1@example.com>",
			Subject:   "Three Node Loop",
			EmailDate: now.Add(-2 * time.Minute),
		},
		{
			MessageID: "<msg3@example.com>",
			InReplyTo: "<msg2@example.com>",
			Subject:   "Three Node Loop",
			EmailDate: now.Add(-1 * time.Minute),
		},
	}

	roots := BuildThreadTree(emails)
	if len(roots) == 0 {
		t.Fatal("expected at least one root node")
	}

	// Must terminate and not panic
	count := CountThreadReplies(roots[0])
	if count < 0 {
		t.Errorf("unexpected count: %d", count)
	}
}

func TestBuildThreadTree_SelfReference(t *testing.T) {
	now := time.Now()
	emails := []Email{
		{
			MessageID: "<self@example.com>",
			InReplyTo: "<self@example.com>",
			Subject:   "Self Reference",
			EmailDate: now,
		},
	}

	roots := BuildThreadTree(emails)
	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}
	if len(roots[0].Children) != 0 {
		t.Errorf("expected 0 children for self-referencing email, got %d", len(roots[0].Children))
	}
}
