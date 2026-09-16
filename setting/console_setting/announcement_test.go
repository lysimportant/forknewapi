package console_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// preserveAnnouncements 保存并恢复公告配置，避免用例相互影响
func preserveAnnouncements(t *testing.T) {
	t.Helper()
	original := GetConsoleSetting().Announcements
	t.Cleanup(func() {
		GetConsoleSetting().Announcements = original
	})
}

func TestGetAnnouncementsOrdersPinnedFirstThenPublishDateDesc(t *testing.T) {
	preserveAnnouncements(t)
	GetConsoleSetting().Announcements = `[
		{"id":1,"content":"normal-newest","publishDate":"2026-09-18T10:00:00Z"},
		{"id":2,"content":"pinned-same-a","publishDate":"2026-09-15T10:00:00Z","pinned":true},
		{"id":3,"content":"normal-oldest","publishDate":"2026-09-14T10:00:00Z"},
		{"id":4,"content":"pinned-same-b","publishDate":"2026-09-15T10:00:00Z","pinned":true},
		{"id":5,"content":"pinned-newest","publishDate":"2026-09-16T10:00:00Z","pinned":true}
	]`

	list := GetAnnouncements()
	require.Len(t, list, 5)

	// 置顶公告优先，置顶之间按发布时间倒序
	assert.Equal(t, "pinned-newest", list[0]["content"])
	// 置顶且发布时间相同时保持配置顺序
	assert.Equal(t, "pinned-same-a", list[1]["content"])
	assert.Equal(t, "pinned-same-b", list[2]["content"])
	// 非置顶公告按发布时间倒序
	assert.Equal(t, "normal-newest", list[3]["content"])
	assert.Equal(t, "normal-oldest", list[4]["content"])
}

func TestAnnouncementsWithoutPinnedReadAsNotPinned(t *testing.T) {
	preserveAnnouncements(t)
	// 历史数据没有 pinned 字段：校验必须通过，排序按未置顶处理
	legacy := `[{"id":1,"content":"legacy","publishDate":"2026-09-15T10:00:00Z"}]`
	require.NoError(t, validateAnnouncements(legacy))
	GetConsoleSetting().Announcements = legacy

	list := GetAnnouncements()
	require.Len(t, list, 1)
	assert.NotContains(t, list[0], "pinned")
	assert.False(t, isPinned(list[0]))
}

func TestValidateAnnouncementsRejectsNonBooleanPinned(t *testing.T) {
	invalid := map[string]string{
		"字符串": `{"content":"c","publishDate":"2026-09-15T10:00:00Z","pinned":"true"}`,
		"数字":  `{"content":"c","publishDate":"2026-09-15T10:00:00Z","pinned":1}`,
		"空值":  `{"content":"c","publishDate":"2026-09-15T10:00:00Z","pinned":null}`,
	}
	for name, item := range invalid {
		t.Run(name, func(t *testing.T) {
			err := validateAnnouncements("[" + item + "]")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "pinned")
		})
	}

	require.NoError(t, validateAnnouncements(`[{"content":"c","publishDate":"2026-09-15T10:00:00Z","pinned":true}]`))
	require.NoError(t, validateAnnouncements(`[{"content":"c","publishDate":"2026-09-15T10:00:00Z","pinned":false}]`))
	// 未知键必须继续透传，不因新增校验而拒绝
	require.NoError(t, validateAnnouncements(`[{"content":"c","publishDate":"2026-09-15T10:00:00Z","unknownKey":"kept"}]`))
}
