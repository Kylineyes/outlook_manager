package graph

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const graphMaxAttempts = 4

// doGraphRequest 对 Graph 请求做有限重试。
// 仅对网络错误、HTTP 429 和 5xx 响应重试，避免重复执行确定失败的业务请求。
func doGraphRequest(req *http.Request) (*http.Response, error) {
	var lastErr error
	for attempt := 1; attempt <= graphMaxAttempts; attempt++ {
		attemptReq := req.Clone(req.Context())
		if req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, fmt.Errorf("重建 Graph 请求体失败: %w", err)
			}
			attemptReq.Body = body
		}
		resp, err := httpClient.Do(attemptReq)
		if err != nil {
			lastErr = err
			if attempt >= graphMaxAttempts || !isRetryableNetworkError(err) {
				return nil, err
			}
			if err := sleepWithContext(req.Context(), retryDelay(attempt, "")); err != nil {
				return nil, err
			}
			continue
		}
		if shouldRetryStatus(resp.StatusCode) && attempt < graphMaxAttempts {
			retryAfter := resp.Header.Get("Retry-After")
			resp.Body.Close()
			if err := sleepWithContext(req.Context(), retryDelay(attempt, retryAfter)); err != nil {
				return nil, err
			}
			continue
		}
		return resp, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("Graph 请求重试次数已用尽")
}

// shouldRetryStatus 判断 HTTP 状态码是否属于可重试的临时错误。
func shouldRetryStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

// isRetryableNetworkError 判断网络错误是否值得重试。
func isRetryableNetworkError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	message := strings.ToLower(err.Error())
	for _, keyword := range []string{"connection reset", "connection refused", "tls handshake timeout", "unexpected eof", "i/o timeout", "wsarecv", "forcibly closed"} {
		if strings.Contains(message, keyword) {
			return true
		}
	}
	return false
}

// retryDelay 计算退避时间，并优先使用服务端返回的 Retry-After。
func retryDelay(attempt int, retryAfter string) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && seconds > 0 {
		if seconds > 30 {
			seconds = 30
		}
		return time.Duration(seconds) * time.Second
	}
	delay := time.Duration(200*(1<<(attempt-1))) * time.Millisecond
	if delay > 2*time.Second {
		delay = 2 * time.Second
	}
	return delay
}

// sleepWithContext 在等待期间响应上下文取消。
func sleepWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
