package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

// NotifyRanking 单次发送并核对 Bot 的显式反馈；结果仅记日志，不持久化或重试。
// 请求失败、超时和旧 Bot 响应都不能证明 Telegram 未发送，不据此再次投递。
func (n *BotNotifier) NotifyRanking(data RankingNotification) {
	requestID := fmt.Sprintf("bot-notify-%d", n.requestSeq.Add(1))
	started := time.Now()
	outcome, reason := "unconfirmed", "invalid_payload"
	status, payloadSize := 0, 0
	defer func() {
		// 不输出 URL、异常原文或响应体，避免通知配置与 Telegram Token 泄露。
		log.Printf("[BotNotifier] endpoint=/notify/ranking event=notify.ranking batchId=%q period=%q requestId=%s outcome=%s reason=%s status=%d payloadSize=%d latency=%s",
			data.BatchID, data.Period, requestID, outcome, reason, status, payloadSize, time.Since(started))
	}()
	if !n.IsConfigured() {
		outcome, reason = "skipped", "bot_not_configured"
		return
	}
	n.mu.RLock()
	botURL := n.botURL
	n.mu.RUnlock()
	body, err := json.Marshal(data)
	if err != nil {
		return
	}
	payloadSize = len(body)
	reason = "invalid_request"
	req, err := http.NewRequest(http.MethodPost, botURL+"/notify/ranking", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Secret", n.secret)
	req.Header.Set("X-Request-Id", requestID)
	reason = "request_error"
	resp, err := n.client.Do(req)
	if err != nil {
		if os.IsTimeout(err) {
			reason = "timeout"
		}
		return
	}
	defer resp.Body.Close()
	status = resp.StatusCode
	reason = "http_error"
	if status != http.StatusOK {
		return
	}
	var result struct {
		OK     bool   `json:"ok"`
		Sent   *bool  `json:"sent"`
		Reason string `json:"reason"`
	}
	reason = "invalid_response"
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&result); err != nil {
		if os.IsTimeout(err) {
			reason = "timeout"
		}
		return
	}
	if !result.OK || result.Sent == nil {
		return
	}
	if *result.Sent {
		outcome, reason = "sent", ""
	} else if result.Reason == "chat_not_configured" {
		outcome, reason = "skipped", "chat_not_configured"
	}
}
