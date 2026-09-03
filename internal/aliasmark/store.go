// Package aliasmark 记录哪些 HME 别名已拿去注册过 Grok,避免反复向 Apple 创建。
package aliasmark

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"icloud-hme/internal/hme"
)

const (
	StatusClaimed = "claimed"
	StatusUsed    = "used"
	defaultTTL    = 45 * time.Minute
)

// Entry 是一个别名的占用/已用记录。
type Entry struct {
	Status      string `json:"status"`
	AnonymousID string `json:"anonymous_id,omitempty"`
	ClaimedAt   string `json:"claimed_at,omitempty"`
	UsedAt      string `json:"used_at,omitempty"`
}

type fileShape struct {
	Accounts map[string]map[string]Entry `json:"accounts"`
}

// Store 按账号+邮箱持久化标记。path 为空时只存在内存。
type Store struct {
	mu        sync.Mutex
	path      string
	ttl       time.Duration
	now       func() time.Time
	accounts  map[string]map[string]Entry
}

// Open 打开标记库。
func Open(path string) *Store {
	s := &Store{
		path:     path,
		ttl:      defaultTTL,
		now:      time.Now,
		accounts: map[string]map[string]Entry{},
	}
	if path == "" {
		return s
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	var wrap fileShape
	if json.Unmarshal(raw, &wrap) == nil && wrap.Accounts != nil {
		s.accounts = wrap.Accounts
	}
	return s
}

// PickUnused 从 Apple 别名列表中挑一个未注册、未占用的活跃别名并标记为 claimed。
func (s *Store) PickUnused(accountID string, aliases []hme.Alias) (hme.Alias, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for _, alias := range aliases {
		if !alias.Active || strings.TrimSpace(alias.Email) == "" {
			continue
		}
		email := strings.ToLower(strings.TrimSpace(alias.Email))
		if s.isBusyLocked(accountID, email, now) {
			continue
		}
		s.putLocked(accountID, email, Entry{
			Status:      StatusClaimed,
			AnonymousID: alias.AnonymousID,
			ClaimedAt:   now.Format(time.RFC3339),
		})
		_ = s.saveLocked()
		return alias, true
	}
	return hme.Alias{}, false
}

// RememberCreated 新创建的别名记为占用中。
func (s *Store) RememberCreated(accountID, email, anonymousID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return
	}
	s.putLocked(accountID, email, Entry{
		Status:      StatusClaimed,
		AnonymousID: anonymousID,
		ClaimedAt:   s.now().Format(time.RFC3339),
	})
	_ = s.saveLocked()
}

// MarkUsed 注册机释放邮箱后标记为已用,避免列表缓存里再次分配。
func (s *Store) MarkUsed(accountID, email, anonymousID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return
	}
	cur := s.getLocked(accountID, email)
	if anonymousID == "" {
		anonymousID = cur.AnonymousID
	}
	s.putLocked(accountID, email, Entry{
		Status:      StatusUsed,
		AnonymousID: anonymousID,
		ClaimedAt:   cur.ClaimedAt,
		UsedAt:      s.now().Format(time.RFC3339),
	})
	_ = s.saveLocked()
}

func (s *Store) isBusyLocked(accountID, email string, now time.Time) bool {
	ent := s.getLocked(accountID, email)
	switch ent.Status {
	case StatusUsed:
		return true
	case StatusClaimed:
		t, err := time.Parse(time.RFC3339, ent.ClaimedAt)
		if err != nil {
			return true
		}
		return now.Sub(t) < s.ttl
	default:
		return false
	}
}

func (s *Store) getLocked(accountID, email string) Entry {
	if s.accounts[accountID] == nil {
		return Entry{}
	}
	return s.accounts[accountID][email]
}

func (s *Store) putLocked(accountID, email string, ent Entry) {
	if s.accounts[accountID] == nil {
		s.accounts[accountID] = map[string]Entry{}
	}
	s.accounts[accountID][email] = ent
}

func (s *Store) saveLocked() error {
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(fileShape{Accounts: s.accounts}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, raw, 0600)
}
