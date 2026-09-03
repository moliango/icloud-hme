package mail

import (
	"reflect"
	"testing"
)

func TestImapUsernames(t *testing.T) {
	got := imapUsernames("name@icloud.com")
	want := []string{"name@icloud.com", "name"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("imapUsernames() = %v, want %v", got, want)
	}
	if got := imapUsernames("user@163.com"); len(got) != 1 || got[0] != "user@163.com" {
		t.Fatalf("第三方邮箱不应拆本地部分: %v", got)
	}
}
