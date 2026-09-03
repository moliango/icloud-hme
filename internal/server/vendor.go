// Package server - 给自动化注册机等外部供应商调用的邮箱生命周期接口。
//
// 三个动作对应临时邮箱的分配 / 收信 / 释放:
//
//	POST   /api/vendor/mailbox   优先复用未注册过 Grok 的已有别名,没有再向 Apple 创建
//	GET    /api/vendor/messages  读取该别名收件箱
//	DELETE /api/vendor/mailbox   标记该别名已用于 Grok,不再分配;不向 Apple 删除
package server

import (
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"icloud-hme/internal/hme"
)

type vendorAllocateReq struct {
	AccountID string `json:"account_id"`
	Label     string `json:"label"`
}

type vendorReleaseReq struct {
	AccountID   string `json:"account_id"`
	Email       string `json:"email"`
	AnonymousID string `json:"anonymous_id"`
}

// vendorAllocateMailboxHandler 分配一个 HME 别名。account_id 可空,空则选第一个可用账号。
func (s *Server) vendorAllocateMailboxHandler(c *gin.Context) {
	var req vendorAllocateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		failCode(c, http.StatusBadRequest, "VALIDATION_ERROR", "参数错误")
		return
	}
	if len([]rune(req.Label)) > 200 {
		failCode(c, http.StatusBadRequest, "VALIDATION_ERROR", "参数错误: label 最长 200 字符")
		return
	}
	accountID, err := s.resolveVendorAccount(req.AccountID)
	if err != nil {
		backendFail(c, err)
		return
	}
	label := strings.TrimSpace(req.Label)
	if label == "" {
		label = "grok-register"
	}

	aliases, listErr := s.be.ListAliases(accountID)
	if listErr != nil {
		log.Printf("vendor allocate 列表失败,将尝试创建 account=%s err=%v", accountID, listErr)
	} else if picked, ok := s.marks.PickUnused(accountID, aliases); ok {
		log.Printf("vendor allocate 复用未注册别名 account=%s email=%s", accountID, picked.Email)
		ok(c, gin.H{
			"email":        picked.Email,
			"anonymous_id": picked.AnonymousID,
			"label":        picked.Label,
			"account_id":   accountID,
			"reused":       true,
		})
		return
	}

	result, err := s.be.CreateAlias(accountID, label)
	if err != nil {
		backendFail(c, err)
		return
	}
	if result == nil || strings.TrimSpace(result.Email) == "" {
		failCode(c, http.StatusBadGateway, "UPSTREAM_FAILURE", "创建邮箱失败")
		return
	}
	anonymousID := strings.TrimSpace(result.AnonymousID)
	if anonymousID == "" {
		if alias, found, lookupErr := s.findAliasByEmail(accountID, result.Email); lookupErr != nil {
			backendFail(c, lookupErr)
			return
		} else if found {
			anonymousID = alias.AnonymousID
		}
	}
	s.marks.RememberCreated(accountID, result.Email, anonymousID)
	log.Printf("vendor allocate 新建别名 account=%s email=%s", accountID, result.Email)
	ok(c, gin.H{
		"email":        result.Email,
		"anonymous_id": anonymousID,
		"label":        result.Label,
		"created_at":   result.CreatedAt,
		"account_id":   accountID,
		"reused":       false,
	})
}

// vendorListMessagesHandler 读取指定别名的邮件,契约与 /api/inbox 一致。
func (s *Server) vendorListMessagesHandler(c *gin.Context) {
	s.listInboxHandler(c)
}

// vendorReleaseMailboxHandler 标记别名已用于 Grok,不再分配。不向 Apple 删除。
func (s *Server) vendorReleaseMailboxHandler(c *gin.Context) {
	var req vendorReleaseReq
	if err := c.ShouldBindJSON(&req); err != nil {
		failCode(c, http.StatusBadRequest, "VALIDATION_ERROR", "参数错误")
		return
	}
	accountID, err := s.resolveVendorAccount(req.AccountID)
	if err != nil {
		backendFail(c, err)
		return
	}
	anonymousID := strings.TrimSpace(req.AnonymousID)
	email := strings.TrimSpace(req.Email)
	if anonymousID == "" && email == "" {
		failCode(c, http.StatusBadRequest, "VALIDATION_ERROR", "参数错误: email 或 anonymous_id 必填")
		return
	}
	if anonymousID == "" && email != "" {
		if alias, found, lookupErr := s.findAliasByEmail(accountID, email); lookupErr == nil && found {
			anonymousID = alias.AnonymousID
		}
	}
	s.marks.MarkUsed(accountID, email, anonymousID)
	log.Printf("vendor release 标记已用于 Grok account=%s email=%s", accountID, email)
	ok(c, gin.H{
		"account_id":   accountID,
		"email":        email,
		"anonymous_id": anonymousID,
		"deleted":      false,
		"marked_used":  true,
	})
}

func (s *Server) resolveVendorAccount(accountID string) (string, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID != "" {
		return accountID, nil
	}
	accounts := s.be.ListAccounts()
	for _, acc := range accounts {
		if acc.Status == "active" && (acc.HasCookies || acc.HasAppPassword) {
			return acc.ID, nil
		}
	}
	for _, acc := range accounts {
		if acc.Status == "active" {
			return acc.ID, nil
		}
	}
	if len(accounts) > 0 {
		return accounts[0].ID, nil
	}
	return "", &BackendError{Status: http.StatusBadRequest, Code: "ACCOUNT_NOT_FOUND", Message: "没有可用的 iCloud 账号"}
}

func (s *Server) findAliasByEmail(accountID, email string) (hme.Alias, bool, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return hme.Alias{}, false, nil
	}
	aliases, err := s.be.ListAliases(accountID)
	if err != nil {
		return hme.Alias{}, false, err
	}
	for _, alias := range aliases {
		if strings.EqualFold(alias.Email, email) {
			return alias, true, nil
		}
	}
	return hme.Alias{}, false, nil
}
