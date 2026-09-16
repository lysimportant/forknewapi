package operation_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateDocsLink(t *testing.T) {
	// 留空表示回退到默认文档地址
	for _, valid := range []string{
		"",
		"   ",
		"https://docs.newapi.pro",
		"https://docs.example.com/guide?from=header#top",
		"http://127.0.0.1:8080/docs",
	} {
		require.NoError(t, ValidateDocsLink(valid), valid)
	}

	cases := map[string]string{
		"相对地址":      "docs.example.com",
		"缺少主机名":     "https://",
		"脚本协议":      "javascript:alert(1)",
		"data 协议":   "data:text/html,<script>alert(1)</script>",
		"非 http 协议": "ftp://docs.example.com",
		"携带用户名":     "https://user@docs.example.com",
		"携带用户名和密码":  "https://user:secret@docs.example.com",
	}
	for name, invalid := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Error(t, ValidateDocsLink(invalid))
		})
	}
}
