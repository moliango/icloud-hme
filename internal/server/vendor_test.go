package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"icloud-hme/internal/account"
	"icloud-hme/internal/hme"
	"icloud-hme/internal/mail"
)

func TestVendorAllocateMailbox(t *testing.T) {
	f := &fakeBackend{
		accounts: []account.Summary{{
			ID: "acc_1", Name: "主号", Status: "active",
			HasCookies: true, HasAppPassword: true,
		}},
		created: &hme.CreateResult{
			Email:       "hide@icloud.com",
			AnonymousID: "anon-1",
			Label:       "grok-register",
			CreatedAt:   "2026-09-02T00:00:00Z",
		},
	}
	_, ts := newTestServer(f)
	defer ts.Close()
	sess, csrf := login(t, ts, "admin-pass-2026-strong")

	req := authedReq(t, ts, "POST", "/api/vendor/mailbox", `{"label":"grok-register"}`)
	req.AddCookie(&http.Cookie{Name: "hme_session", Value: sess})
	req.Header.Set("X-CSRF-Token", csrf)
	status, body, _ := do(t, req)
	if status != http.StatusOK {
		t.Fatalf("期望 200,得到 %d: %s", status, body)
	}
	if f.createAccountID != "acc_1" {
		t.Fatalf("应自动选择 active 账号,得到 %q", f.createAccountID)
	}
	var out struct {
		Success bool `json:"success"`
		Data    struct {
			Email       string `json:"email"`
			AnonymousID string `json:"anonymous_id"`
			AccountID   string `json:"account_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if out.Data.Email != "hide@icloud.com" || out.Data.AnonymousID != "anon-1" || out.Data.AccountID != "acc_1" {
		t.Fatalf("分配响应不正确: %+v", out.Data)
	}
}

func TestVendorAllocateLooksUpAnonymousID(t *testing.T) {
	f := &fakeBackend{
		accounts: []account.Summary{{ID: "acc_1", Status: "active", HasCookies: true}},
		created:  &hme.CreateResult{Email: "hide@icloud.com", Label: "x"},
		aliases: []hme.Alias{{
			Email: "hide@icloud.com", AnonymousID: "looked-up", Active: true,
		}},
	}
	_, ts := newTestServer(f)
	defer ts.Close()
	sess, csrf := login(t, ts, "admin-pass-2026-strong")

	req := authedReq(t, ts, "POST", "/api/vendor/mailbox", `{"account_id":"acc_1"}`)
	req.AddCookie(&http.Cookie{Name: "hme_session", Value: sess})
	req.Header.Set("X-CSRF-Token", csrf)
	status, body, _ := do(t, req)
	if status != http.StatusOK {
		t.Fatalf("期望 200,得到 %d: %s", status, body)
	}
	if !strings.Contains(body, `"anonymous_id":"looked-up"`) {
		t.Fatalf("应回填 anonymous_id: %s", body)
	}
}

func TestVendorMessagesAndRelease(t *testing.T) {
	f := &fakeBackend{
		accounts: []account.Summary{{ID: "acc_1", Status: "active", HasCookies: true}},
		aliases: []hme.Alias{{
			Email: "hide@icloud.com", AnonymousID: "anon-9", Active: true,
		}},
		inbox: InboxResult{
			AccountID: "acc_1",
			Alias:     "hide@icloud.com",
			Count:     1,
			Method:    "imap",
			Messages: []mail.Message{{
				ID: "1", From: "noreply@x.ai", To: "hide@icloud.com",
				Subject: "ABC-123 xAI", Preview: "Your code is ABC-123",
			}},
		},
	}
	_, ts := newTestServer(f)
	defer ts.Close()
	sess, csrf := login(t, ts, "admin-pass-2026-strong")

	req := authedReq(t, ts, "GET", "/api/vendor/messages?account_id=acc_1&alias=hide@icloud.com", "")
	req.AddCookie(&http.Cookie{Name: "hme_session", Value: sess})
	status, body, _ := do(t, req)
	if status != http.StatusOK {
		t.Fatalf("收信期望 200,得到 %d: %s", status, body)
	}
	if f.listInboxQuery.Alias != "hide@icloud.com" {
		t.Fatalf("应收指定别名,得到 %+v", f.listInboxQuery)
	}
	if !strings.Contains(body, `"ABC-123 xAI"`) {
		t.Fatalf("应返回验证码邮件: %s", body)
	}

	req = authedReq(t, ts, "DELETE", "/api/vendor/mailbox", `{"account_id":"acc_1","email":"hide@icloud.com"}`)
	req.AddCookie(&http.Cookie{Name: "hme_session", Value: sess})
	req.Header.Set("X-CSRF-Token", csrf)
	status, body, _ = do(t, req)
	if status != http.StatusOK {
		t.Fatalf("释放期望 200,得到 %d: %s", status, body)
	}
	if f.aliasDeleteID != "anon-9" {
		t.Fatalf("应按邮箱解析 anonymous_id 后删除,得到 %q", f.aliasDeleteID)
	}
}

func TestVendorAllocateRequiresAccount(t *testing.T) {
	f := &fakeBackend{}
	_, ts := newTestServer(f)
	defer ts.Close()
	sess, csrf := login(t, ts, "admin-pass-2026-strong")

	req := authedReq(t, ts, "POST", "/api/vendor/mailbox", `{}`)
	req.AddCookie(&http.Cookie{Name: "hme_session", Value: sess})
	req.Header.Set("X-CSRF-Token", csrf)
	status, body, _ := do(t, req)
	if status != http.StatusBadRequest {
		t.Fatalf("无账号期望 400,得到 %d: %s", status, body)
	}
}
