package ask

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Cloudflare runs prompts on Workers AI. Credentials come from the environment
// or ~/cloudflare_ai_token.env (CLOUDFLARE_AI_TOKEN, CLOUDFLARE_ACCOUNT_ID).
type Cloudflare struct {
	AccountID string
	Token     string
	Model     string
	Client    *http.Client
}

const DefaultCloudflareModel = "@cf/meta/llama-3.1-8b-instruct-fast"

// CloudflareFromEnv returns a configured client, or ok=false when the account id or token is missing.
func CloudflareFromEnv() (Cloudflare, bool) {
	vals := map[string]string{}
	home, _ := os.UserHomeDir()
	for _, path := range []string{os.Getenv("KENNEL_CF_ENV"), filepath.Join(home, "cloudflare_ai_token.env")} {
		if path == "" {
			continue
		}
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
			if ok && !strings.HasPrefix(k, "#") {
				vals[strings.TrimPrefix(strings.TrimSpace(k), "export ")] = strings.Trim(strings.TrimSpace(v), `"'`)
			}
		}
		f.Close()
	}
	get := func(k string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		return vals[k]
	}
	c := Cloudflare{AccountID: get("CLOUDFLARE_ACCOUNT_ID"), Token: get("CLOUDFLARE_AI_TOKEN"), Model: get("KENNEL_CF_MODEL")}
	if c.Token == "" {
		c.Token = get("CLOUDFLARE_API_TOKEN")
	}
	if c.Model == "" {
		c.Model = DefaultCloudflareModel
	}
	return c, c.AccountID != "" && c.Token != ""
}

func (c Cloudflare) Complete(ctx context.Context, _ string, prompt string, onText func(string)) (string, error) {
	body, _ := json.Marshal(map[string]any{"messages": []map[string]string{{"role": "user", "content": prompt}}, "max_tokens": 2048})
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/ai/run/%s", c.AccountID, c.Model)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	var out struct {
		Success bool `json:"success"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Result struct {
			Response string `json:"response"`
		} `json:"result"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("cloudflare: %s", res.Status)
	}
	if !out.Success {
		msg := res.Status
		if len(out.Errors) > 0 {
			msg = out.Errors[0].Message
		}
		return "", fmt.Errorf("cloudflare: %s", msg)
	}
	if onText != nil {
		onText(out.Result.Response)
	}
	return out.Result.Response, nil
}
