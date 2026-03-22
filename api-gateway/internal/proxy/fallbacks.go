package proxy

import (
	"errors"
	"net"
	"net/http"
)

func shouldAttemptFallback(statusCode int, err error) bool {
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) {
			return true
		}
		return true
	}
	switch statusCode {
	case http.StatusRequestTimeout, http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return statusCode >= 500
	}
}

func classifyUpstreamFailure(statusCode int, err error) string {
	if err != nil {
		return "upstream_request_failed"
	}
	switch statusCode {
	case http.StatusTooManyRequests:
		return "rate_limited"
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return "upstream_unavailable"
	default:
		if statusCode >= 500 {
			return "upstream_error"
		}
		if statusCode >= 400 {
			return "client_error"
		}
		return ""
	}
}
