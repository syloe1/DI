package core

import (
	"log"
	"os"
)

/*
log.LstdFlags：等价于 Ldate | Ltime，输出 日期 + 时分秒
log.Lshortfile：输出 代码文件名 + 行号，快速定位日志所在代码位置
*/
func NewLogger() *log.Logger {
	// 新建日志对象
	return log.New(
		os.Stdout,                    // 日志输出目标：标准控制台
		"[go-admin] ",                // 日志前缀，每条日志开头都会带上
		log.LstdFlags|log.Lshortfile, // 日志输出标记组合
	)
}
