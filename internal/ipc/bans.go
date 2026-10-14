package ipc

import (
	"net"
	"sync"
	"time"
)

const (
	maxFailedAuthorizationAttempts = 5
	failedAuthorizationsCooldown   = 1 * time.Hour
)

var (
	failedMu             sync.Mutex
	failedAuthorizations = make(map[string]int)
)

func init() {
	go func() {
		ticker := time.NewTicker(failedAuthorizationsCooldown)
		defer ticker.Stop()
		for range ticker.C {
			clearFailedAuthorizations()
		}
	}()
}

func clientIP(remoteAddr string) string {
	ip, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return ip
}

func isBanned(ip string) bool {
	failedMu.Lock()
	defer failedMu.Unlock()
	return failedAuthorizations[ip] >= maxFailedAuthorizationAttempts
}

func recordFailure(ip string) int {
	failedMu.Lock()
	defer failedMu.Unlock()
	failedAuthorizations[ip]++
	return failedAuthorizations[ip]
}

func getCurrentlyBannedIPs() []string {
	failedMu.Lock()
	defer failedMu.Unlock()
	out := make([]string, 0)
	for ip, n := range failedAuthorizations {
		if n >= maxFailedAuthorizationAttempts {
			out = append(out, ip)
		}
	}
	return out
}

func unbanIP(ip string) bool {
	failedMu.Lock()
	defer failedMu.Unlock()
	if failedAuthorizations[ip] >= maxFailedAuthorizationAttempts {
		delete(failedAuthorizations, ip)
		return true
	}
	return false
}

func clearFailedAuthorizations() {
	failedMu.Lock()
	defer failedMu.Unlock()
	failedAuthorizations = make(map[string]int)
}
