package core

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestNewBizError(t *testing.T) {
	err := NewBizError(http.StatusBadRequest, "bad request")

	if err.Code != http.StatusBadRequest {
		t.Errorf("Code = %d, want %d", err.Code, http.StatusBadRequest)
	}
	if err.Msg != "bad request" {
		t.Errorf("Msg = %q, want %q", err.Msg, "bad request")
	}
	// BizError 必须实现 error 接口
	var e error = err
	if e.Error() != "bad request" {
		t.Errorf("Error() = %q, want %q", e.Error(), "bad request")
	}
}

// newTestRouter 构造一个把 err 交给 FailByError 的测试路由。
func newTestRouter(err error) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/t", func(c *gin.Context) {
		FailByError(c, err)
	})
	return r
}

func TestFailByError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantBody   string // 只断言响应体包含的片段
	}{
		{
			name:       "BizError 透传 code 与 msg",
			err:        NewBizError(http.StatusUnauthorized, "need login"),
			wantStatus: http.StatusUnauthorized,
			wantBody:   `"code":401`,
		},
		{
			name:       "BizError 自定义文案",
			err:        NewBizError(http.StatusForbidden, "no permission"),
			wantStatus: http.StatusForbidden,
			wantBody:   "no permission",
		},
		{
			name:       "普通 error 归一化为 500 internal error（不泄漏内部细节）",
			err:        errors.New("dial tcp 127.0.0.1:3306: connect: connection refused"),
			wantStatus: http.StatusInternalServerError,
			wantBody:   "internal error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestRouter(tt.err)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/t", nil)
			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			body := w.Body.String()
			if !strings.Contains(body, tt.wantBody) {
				t.Errorf("body = %q, want it to contain %q", body, tt.wantBody)
			}
			// 非 BizError 的内部错误：原始错误文本绝不回显给客户端（防 SQL/路径泄漏）
			if _, isBiz := tt.err.(*BizError); !isBiz && strings.Contains(body, "connection refused") {
				t.Errorf("internal error detail leaked to client: %q", body)
			}
		})
	}
}

func TestRespond_EmptyMsgFallsBackToStatusText(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/t", func(c *gin.Context) {
		Respond(c, http.StatusNotFound, "", nil)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
	if !strings.Contains(w.Body.String(), "Not Found") {
		t.Errorf("body = %q, want it to contain status text", w.Body.String())
	}
}
