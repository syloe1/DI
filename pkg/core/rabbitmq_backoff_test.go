package core

import (
	"testing"
	"time"
)

// TestNextReconnectBackoff 表驱动：验证指数退避的边界（1s 起步、×2、30s 封顶）。
func TestNextReconnectBackoff(t *testing.T) {
	tests := []struct {
		name    string
		current time.Duration
		want    time.Duration
	}{
		{name: "初始值 1s 起步", current: 0, want: time.Second},
		{name: "低于 1s 归一到 1s", current: 500 * time.Millisecond, want: time.Second},
		{name: "1s 翻倍为 2s", current: time.Second, want: 2 * time.Second},
		{name: "4s 翻倍为 8s", current: 4 * time.Second, want: 8 * time.Second},
		{name: "15s 翻倍为 30s", current: 15 * time.Second, want: 30 * time.Second},
		{name: "达到 30s 封顶不再增长", current: 30 * time.Second, want: 30 * time.Second},
		{name: "超过上限也封顶", current: 45 * time.Second, want: 30 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nextReconnectBackoff(tt.current)
			if got != tt.want {
				t.Errorf("nextReconnectBackoff(%v) = %v, want %v", tt.current, got, tt.want)
			}
		})
	}
}
