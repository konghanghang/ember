// Package async 提供火忘式后台任务的安全封装。
//
// 业务里大量"通知 Bot / 推送排行榜 / 注册成功事件"这类副作用调用，
// 不希望阻塞主流程，但裸 `go fn()` 一旦 fn panic 会直接 crash 整个 API 进程。
//
// SafeGo 在独立 goroutine 中执行 fn，捕获 panic 并以结构化日志记录调用点、
// 耗时与堆栈，调用方主流程不会受影响。
package async

import (
	"log"
	"runtime/debug"
	"time"
)

// SafeGo 在新的 goroutine 中执行 fn。
//
// name 用来在日志中标识调用点，例如 "subscription.notifyApproved"，
// 出现 panic 或异常耗时时便于按 name 聚合排查。
//
// fn 内部出现的 panic 会被 recover 住，不会向上传播。
func SafeGo(name string, fn func()) {
	if fn == nil {
		return
	}
	go func() {
		start := time.Now()
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[async.SafeGo] panic name=%s elapsed=%s recovered=%v\nstack=%s",
					name, time.Since(start), r, debug.Stack())
			}
		}()
		fn()
	}()
}
