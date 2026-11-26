package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var ErrAIConfig = errors.New("invalid AI configuration")

type AI struct {
	BaseURL string
	APIKey  string
	Model   string
	Timeout time.Duration
}

// AI configuration errors disable analysis only. Database tools do not load it.
func LoadAI() (AI, error) {
	timeout, err := time.ParseDuration(envDefault("AI_TIMEOUT", "20s"))
	if err != nil {
		return AI{}, fmt.Errorf("%w: AI_TIMEOUT must be a positive duration", ErrAIConfig)
	}
	cfg := AI{BaseURL: os.Getenv("AI_BASE_URL"), APIKey: os.Getenv("AI_API_KEY"), Model: os.Getenv("AI_MODEL"), Timeout: timeout}
	if _, err := cfg.Endpoint(); err != nil {
		return AI{}, err
	}
	return cfg, nil
}

// Endpoint validates typed configurations as well as environment configuration.
// Errors identify the field without including its value.
func (c AI) Endpoint() (string, error) {
	if c.Timeout <= 0 {
		return "", fmt.Errorf("%w: AI_TIMEOUT must be a positive duration", ErrAIConfig)
	}
	if !utf8.ValidString(c.Model) || utf8.RuneCountInString(c.Model) > 255 || strings.TrimSpace(c.Model) == "" || strings.TrimSpace(c.Model) != c.Model || strings.IndexFunc(c.Model, unicode.IsControl) >= 0 {
		return "", fmt.Errorf("%w: AI_MODEL must be a nonblank model name", ErrAIConfig)
	}
	if c.APIKey == "" || strings.IndexFunc(c.APIKey, func(r rune) bool { return r < 33 || r > 126 }) >= 0 {
		return "", fmt.Errorf("%w: AI_API_KEY must be a nonempty bearer credential", ErrAIConfig)
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Hostname() == "" || u.User != nil || strings.ContainsAny(c.BaseURL, "?#") {
		return "", fmt.Errorf("%w: AI_BASE_URL requires a host and no credentials, query or fragment", ErrAIConfig)
	}
	if port := u.Port(); port != "" {
		n, err := strconv.ParseUint(port, 10, 16)
		if err != nil || n == 0 {
			return "", fmt.Errorf("%w: invalid AI_BASE_URL port", ErrAIConfig)
		}
	} else if strings.HasSuffix(u.Host, ":") {
		return "", fmt.Errorf("%w: invalid AI_BASE_URL port", ErrAIConfig)
	}
	local := strings.EqualFold(u.Hostname(), "localhost") || net.ParseIP(u.Hostname()).IsLoopback()
	if u.Scheme != "https" && !(u.Scheme == "http" && local) {
		return "", fmt.Errorf("%w: AI_BASE_URL requires HTTPS or loopback HTTP", ErrAIConfig)
	}
	base := strings.TrimRight(u.String(), "/")
	if strings.HasSuffix(base, "/chat/completions") {
		return base, nil
	}
	return base + "/chat/completions", nil
}
