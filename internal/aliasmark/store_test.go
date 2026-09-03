package aliasmark

import (
	"path/filepath"
	"testing"
	"time"

	"icloud-hme/internal/hme"
)

func TestPickUnusedSkipsUsedAndClaimed(t *testing.T) {
	s := Open("")
	s.now = func() time.Time { return time.Date(2026, 9, 3, 4, 0, 0, 0, time.UTC) }
	aliases := []hme.Alias{
		{Email: "used@icloud.com", AnonymousID: "a1", Active: true},
		{Email: "claimed@icloud.com", AnonymousID: "a2", Active: true},
		{Email: "free@icloud.com", AnonymousID: "a3", Active: true},
		{Email: "dead@icloud.com", AnonymousID: "a4", Active: false},
	}
	s.MarkUsed("acc_1", "used@icloud.com", "a1")
	s.RememberCreated("acc_1", "claimed@icloud.com", "a2")

	got, ok := s.PickUnused("acc_1", aliases)
	if !ok || got.Email != "free@icloud.com" {
		t.Fatalf("应复用未标记别名,得到 ok=%v %+v", ok, got)
	}
	got, ok = s.PickUnused("acc_1", aliases)
	if ok {
		t.Fatalf("不应再分出已占用别名: %+v", got)
	}
}

func TestClaimExpires(t *testing.T) {
	s := Open("")
	start := time.Date(2026, 9, 3, 4, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return start }
	s.RememberCreated("acc_1", "old@icloud.com", "x")
	s.now = func() time.Time { return start.Add(s.ttl + time.Minute) }
	got, ok := s.PickUnused("acc_1", []hme.Alias{
		{Email: "old@icloud.com", AnonymousID: "x", Active: true},
	})
	if !ok || got.Email != "old@icloud.com" {
		t.Fatalf("占用过期应可再分配,得到 ok=%v %+v", ok, got)
	}
}

func TestPersistRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "alias_marks.json")
	s := Open(path)
	s.MarkUsed("acc_1", "done@icloud.com", "z")
	s2 := Open(path)
	_, ok := s2.PickUnused("acc_1", []hme.Alias{
		{Email: "done@icloud.com", AnonymousID: "z", Active: true},
	})
	if ok {
		t.Fatal("已用标记应持久化,不能再分配")
	}
}
