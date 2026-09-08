package palm

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPaLMChatRequestContract 验证 generateMessage 的上下文、消息顺序和作者映射，保留可选标量显式零值。
func TestPaLMChatRequestContract(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{
		Model: "chat-bison-001", Temperature: common.GetPointer(0.0), TopP: common.GetPointer(0.0), TopK: common.GetPointer(0), N: common.GetPointer(0),
		Messages: []dto.Message{
			{Role: "system", Content: "system rules"},
			{Role: "developer", Content: "developer rules"},
			{Role: "user", Content: "first"},
			{Role: "assistant", Content: "reply"},
			{Role: "user", Content: "next"},
		},
	}
	converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, nil, request)
	require.NoError(t, err)
	raw, err := common.Marshal(converted)
	require.NoError(t, err)
	assert.JSONEq(t, `{"prompt":{"context":"system rules\n\ndeveloper rules","messages":[{"author":"0","content":"first"},{"author":"1","content":"reply"},{"author":"0","content":"next"}]},"temperature":0,"topP":0,"topK":0,"candidateCount":0}`, string(raw))
	assert.Equal(t, "system", request.Messages[0].Role)
	assert.Len(t, request.Messages, 5)
}

// TestPaLMChatRequestOptionalFields 验证未提供的参数不会被填成默认值或复制成错误的 Chat 顶层字段。
func TestPaLMChatRequestOptionalFields(t *testing.T) {
	converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, nil, &dto.GeneralOpenAIRequest{Messages: []dto.Message{{Role: "user", Content: "hello"}}})
	require.NoError(t, err)
	raw, err := common.Marshal(converted)
	require.NoError(t, err)
	assert.JSONEq(t, `{"prompt":{"messages":[{"author":"0","content":"hello"}]}}`, string(raw))
	_, err = (&Adaptor{}).ConvertOpenAIRequest(nil, nil, nil)
	require.Error(t, err)
}
