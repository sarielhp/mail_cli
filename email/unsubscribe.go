package email

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// UnsubscriptionEntry stores metadata about an unsubscribe attempt.
type UnsubscriptionEntry struct {
	Timestamp   string `json:"timestamp"`
	Sender      string `json:"sender"`
	Method      string `json:"method"`
	Destination string `json:"destination"`
	Success     bool   `json:"success"`
	Error       string `json:"error,omitempty"`
}

// ParseListUnsubscribe parses a List-Unsubscribe header and extracts mailto and HTTP links.
func ParseListUnsubscribe(headerVal string) (string, string) {
	var mailtoLink, httpLink string
	parts := strings.Split(headerVal, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if !strings.HasPrefix(part, "<") || !strings.HasSuffix(part, ">") {
			continue
		}
		link := strings.TrimSpace(part[1 : len(part)-1])
		switch {
		case strings.HasPrefix(link, "mailto:"):
			mailtoLink = link
		case strings.HasPrefix(link, "http://") || strings.HasPrefix(link, "https://"):
			httpLink = link
		}
	}
	return mailtoLink, httpLink
}

// isBlockedIP reports whether ip is a loopback, private, link-local, or unspecified address.
func isBlockedIP(ip net.IP, allowLoopback bool) bool {
	if ip == nil {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	if !allowLoopback && ip.IsLoopback() {
		return true
	}
	return ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}

// validateUnsubscribeURL validates that u uses http/https and does not target prohibited hosts or IPs.
func validateUnsubscribeURL(u *url.URL, allowLoopback bool) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported scheme %q; only http and https are allowed", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return errors.New("empty host in unsubscribe URL")
	}
	if !allowLoopback && (strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost")) {
		return errors.New("blocked host: localhost is prohibited")
	}
	ip := net.ParseIP(host)
	if ip != nil && isBlockedIP(ip, allowLoopback) {
		return fmt.Errorf("blocked address %s: loopback, private, or link-local targets are prohibited", host)
	}
	return nil
}

// safeDialControl returns a socket control callback enforcing destination IP constraints.
func safeDialControl(allowLoopback bool) func(string, string, syscall.RawConn) error {
	return func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			host = address
		}
		ip := net.ParseIP(host)
		if ip == nil || isBlockedIP(ip, allowLoopback) {
			return fmt.Errorf("blocked address %s: loopback, private, or link-local targets are prohibited", address)
		}
		return nil
	}
}

// newSafeHTTPClient constructs an HTTP client protected against SSRF and redirect loops.
func newSafeHTTPClient(allowLoopback bool) *http.Client {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 10 * time.Second,
		Control:   safeDialControl(allowLoopback),
	}
	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		ResponseHeaderTimeout: 10 * time.Second,
	}
	return &http.Client{
		Timeout:   10 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("stopped after 5 redirects")
			}
			return validateUnsubscribeURL(req.URL, allowLoopback)
		},
	}
}

var defaultUnsubClient = newSafeHTTPClient(false)

// ExecuteHTTPUnsubscribe executes a GET request against an unsubscription URL with SSRF protection.
func ExecuteHTTPUnsubscribe(urlStr string) error {
	return executeHTTPUnsubscribeWithClient(defaultUnsubClient, urlStr, false)
}

func executeHTTPUnsubscribeWithClient(client *http.Client, urlStr string, allowLoopback bool) error {
	parsed, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid unsubscribe URL: %w", err)
	}
	if validErr := validateUnsubscribeURL(parsed, allowLoopback); validErr != nil {
		return validErr
	}
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP unsubscribe returned status %d: %s", resp.StatusCode, resp.Status)
	}
	return nil
}

// LogUnsubscription appends an unsubscription attempt to the unsubscriptions.json log.
func LogUnsubscription(configDir, sender, method, destination string, unsubErr error) {
	if configDir == "" {
		return
	}
	logPath := filepath.Join(configDir, "unsubscriptions.json")

	var entries []UnsubscriptionEntry
	if data, err := os.ReadFile(logPath); err == nil {
		_ = json.Unmarshal(data, &entries)
	}

	errStr := ""
	if unsubErr != nil {
		errStr = unsubErr.Error()
	}

	entry := UnsubscriptionEntry{
		Timestamp:   time.Now().Format(time.RFC3339),
		Sender:      sender,
		Method:      method,
		Destination: destination,
		Success:     unsubErr == nil,
		Error:       errStr,
	}

	entries = append(entries, entry)

	if bytes, err := json.MarshalIndent(entries, "", "  "); err == nil {
		_ = os.WriteFile(logPath, bytes, 0600)
	}
}
