package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// BrowserCanvasTool returns a stub response for canvas capture requests.
type BrowserCanvasTool struct {
	sandbox PathSandbox
}

const browserCanvasSandboxHint = "agents.runtime.sandboxRoots"

// NewBrowserCanvasTool creates a new browser.canvas tool.
func NewBrowserCanvasTool(sandbox PathSandbox) Tool {
	return BrowserCanvasTool{sandbox: sandbox}
}

// Name returns the tool name.
func (BrowserCanvasTool) Name() string {
	return "browser.canvas"
}

// Execute validates args and returns a stub response.
func (t BrowserCanvasTool) Execute(ctx context.Context, req ToolRequest) (string, error) {
	_ = ctx
	if len(t.sandbox.Roots) == 0 {
		return "", fmt.Errorf("sandbox roots are not configured (set %s)", browserCanvasSandboxHint)
	}
	trimmed := strings.TrimSpace(req.Args)
	if trimmed == "" {
		return "", fmt.Errorf("args are required")
	}
	var args browserCanvasArgs
	if err := json.Unmarshal([]byte(trimmed), &args); err != nil {
		return "", fmt.Errorf("invalid args")
	}
	host, err := validateCanvasURL(args.URL)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"unsupported: browser.canvas not wired (host=%s width=%d height=%d)",
		host,
		args.Width,
		args.Height,
	), nil
}

type browserCanvasArgs struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

func validateCanvasURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("url is required")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid url")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("url scheme must be http or https")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("url host is required")
	}
	return parsed.Host, nil
}
