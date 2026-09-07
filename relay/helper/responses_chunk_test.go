package helper

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// responsesChunkWriter 模拟完整写、短写与失败写，验证 SSE 发送边界。
type responsesChunkWriter struct {
	gin.ResponseWriter
	// writeErr 配置模拟的网络错误。
	writeErr error
	// short 配置无错误的短写。
	short bool
}

// Write 按测试配置返回写入结果；短写用于验证没有 error 时仍不能报告发送成功。
func (w *responsesChunkWriter) Write(data []byte) (int, error) {
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	if w.short {
		return len(data) - 1, nil
	}
	return w.ResponseWriter.Write(data)
}

// WriteString 使字符串写入复用可控制的 Write 边界。
func (w *responsesChunkWriter) WriteString(data string) (int, error) {
	return w.Write([]byte(data))
}

// TestResponseChunkDataWriteContract 验证 SSE 格式、原有回车转义和实际写错误传播。
func TestResponseChunkDataWriteContract(t *testing.T) {
	writeErr := errors.New("socket failed")
	for _, test := range []struct {
		name     string
		writeErr error
		short    bool
		wantErr  error
	}{
		{name: "success"},
		{name: "write failure", writeErr: writeErr, wantErr: writeErr},
		{name: "short write", short: true, wantErr: io.ErrShortWrite},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Writer = &responsesChunkWriter{ResponseWriter: c.Writer, writeErr: test.writeErr, short: test.short}
			err := ResponseChunkData(c, dto.ResponsesStreamResponse{Type: "response.output_text.delta"}, "{\"type\":\"response.output_text.delta\"}\r")
			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
				assert.False(t, recorder.Flushed)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\"}\\r\n\n", recorder.Body.String())
			assert.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
			assert.Equal(t, "no-cache", recorder.Header().Get("Cache-Control"))
			assert.True(t, recorder.Flushed)
		})
	}
}
