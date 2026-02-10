package client

import (
	"crypto/rand"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// Client wraps an HTTP client with session (cookie) management for hidexx.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// New creates a new Client with cookie jar enabled.
func New(baseURL string) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("create cookie jar: %w", err)
	}

	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTPClient: &http.Client{
			Jar: jar,
		},
	}, nil
}

// Login performs login and returns nil on success.
func (c *Client) Login(email, password string) error {
	loginURL := c.BaseURL + "/users/login"

	form := url.Values{
		"email":    {email},
		"password": {password},
	}

	resp, err := c.HTTPClient.Post(loginURL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("post login: %w", err)
	}
	defer resp.Body.Close()

	// 跟随重定向后，检查最终落地的 URL
	finalURL := resp.Request.URL.String()
	if !strings.Contains(finalURL, "/users/login") {
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	bodyStr := string(body)

	if strings.Contains(bodyStr, "用户名或密码错误") {
		return fmt.Errorf("login failed: wrong email or password")
	}

	return fmt.Errorf("login failed: unexpected response (status=%d)", resp.StatusCode)
}

// Register creates a new account. It handles captcha via OCR with retries.
// On success, the client session is authenticated (auto-login after register).
func (c *Client) Register(email, password string) error {
	const maxAttempts = 10

	for i := 0; i < maxAttempts; i++ {
		// 1. 访问注册页，建立 session
		if _, err := c.HTTPClient.Get(c.BaseURL + "/users/register"); err != nil {
			return fmt.Errorf("get register page: %w", err)
		}

		// 2. 获取验证码图片
		code, err := c.solveCaptcha()
		if err != nil {
			fmt.Printf("  attempt %d: captcha OCR failed: %v, retrying...\n", i+1, err)
			continue
		}
		fmt.Printf("  attempt %d: captcha recognized as '%s'\n", i+1, code)

		// 3. 提交注册
		form := url.Values{
			"email":     {email},
			"pass1":     {password},
			"pass2":     {password},
			"checkcode": {code},
		}

		resp, err := c.HTTPClient.Post(
			c.BaseURL+"/users/register",
			"application/x-www-form-urlencoded",
			strings.NewReader(form.Encode()),
		)
		if err != nil {
			return fmt.Errorf("post register: %w", err)
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		finalURL := resp.Request.URL.String()
		bodyStr := string(body)

		// 注册成功 → 自动跳转到 ucenter
		if strings.Contains(finalURL, "/users/ucenter") {
			return nil
		}

		// 验证码错误 → 重试
		if strings.Contains(bodyStr, "验证码") {
			fmt.Printf("  attempt %d: captcha incorrect, retrying...\n", i+1)
			continue
		}

		// 邮箱已注册
		if strings.Contains(bodyStr, "已注册") || strings.Contains(bodyStr, "已存在") {
			return fmt.Errorf("email already registered: %s", email)
		}

		return fmt.Errorf("register failed: unexpected response (url=%s)", finalURL)
	}

	return fmt.Errorf("register failed: captcha not solved after %d attempts", maxAttempts)
}

// solveCaptcha downloads the captcha image and runs tesseract OCR.
func (c *Client) solveCaptcha() (string, error) {
	resp, err := c.HTTPClient.Get(c.BaseURL + "/users/vcode")
	if err != nil {
		return "", fmt.Errorf("get captcha: %w", err)
	}
	defer resp.Body.Close()

	// 写入临时文件
	tmpFile, err := os.CreateTemp("", "hidexx-captcha-*.png")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("save captcha: %w", err)
	}
	tmpFile.Close()

	// 调用 tesseract OCR
	out, err := exec.Command("tesseract", tmpPath, "stdout").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tesseract: %w (output: %s)", err, string(out))
	}

	code := strings.TrimSpace(string(out))
	// 去除空格
	code = strings.ReplaceAll(code, " ", "")

	// 验证码必须是 4 位
	if len(code) != 4 {
		return "", fmt.Errorf("OCR result '%s' is not 4 chars", code)
	}

	return code, nil
}

// GenerateRandomAccount generates a random email and password.
func GenerateRandomAccount() (email, password string) {
	suffix := randomString(8, "abcdefghijklmnopqrstuvwxyz0123456789")
	email = fmt.Sprintf("hx%s@outlook.com", suffix)
	password = fmt.Sprintf("Hx@%s", randomString(10, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"))
	return
}

func randomString(n int, charset string) string {
	b := make([]byte, n)
	for i := range b {
		idx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		b[i] = charset[idx.Int64()]
	}
	return string(b)
}

// trialParams holds the dynamic parameters parsed from the user center page.
type trialParams struct {
	SID      string
	Checksum string
}

var (
	reSID      = regexp.MustCompile(`name=['"]?sid['"]?\s+value=['"]?([^'">\s]+)`)
	reChecksum = regexp.MustCompile(`name=['"]?checksum['"]?\s+value=['"]?([^'">\s]+)`)
	reSubLink  = regexp.MustCompile(`copyText\(['"]([^'"]+)['"]\)`)
)

// fetchUcenterHTML fetches and returns the user center page HTML.
func (c *Client) fetchUcenterHTML() (string, error) {
	resp, err := c.HTTPClient.Get(c.BaseURL + "/users/ucenter")
	if err != nil {
		return "", fmt.Errorf("get ucenter: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read ucenter: %w", err)
	}
	return string(body), nil
}

// parseTrialParams fetches the user center page and extracts sid/checksum.
func (c *Client) parseTrialParams() (*trialParams, error) {
	html, err := c.fetchUcenterHTML()
	if err != nil {
		return nil, err
	}

	sidMatch := reSID.FindStringSubmatch(html)
	if sidMatch == nil {
		return nil, fmt.Errorf("sid not found in ucenter page (not logged in?)")
	}

	checksumMatch := reChecksum.FindStringSubmatch(html)
	if checksumMatch == nil {
		return nil, fmt.Errorf("checksum not found in ucenter page")
	}

	return &trialParams{
		SID:      sidMatch[1],
		Checksum: checksumMatch[1],
	}, nil
}

// Subscription holds a subscription link with its label.
type Subscription struct {
	Label string
	URL   string
}

// GetSubscriptions fetches the user center page and extracts all subscription links.
func (c *Client) GetSubscriptions() ([]Subscription, error) {
	html, err := c.fetchUcenterHTML()
	if err != nil {
		return nil, err
	}

	// 匹配 onclick="copyText('https://...')" 和其前面的文本标签
	// 标签在 HTML 中的格式: <div ... onclick="copyText('URL')">Label</div>
	reLabeledLink := regexp.MustCompile(`onclick="copyText\('([^']+)'\)"[^>]*>([^<]+)<`)
	matches := reLabeledLink.FindAllStringSubmatch(html, -1)

	if len(matches) == 0 {
		// fallback: 只提取 URL
		urlMatches := reSubLink.FindAllStringSubmatch(html, -1)
		if len(urlMatches) == 0 {
			return nil, fmt.Errorf("no subscription links found (no active plan?)")
		}
		var subs []Subscription
		for _, m := range urlMatches {
			subs = append(subs, Subscription{URL: strings.ReplaceAll(m[1], "&amp;", "&")})
		}
		return subs, nil
	}

	var subs []Subscription
	for _, m := range matches {
		subs = append(subs, Subscription{
			URL:   strings.ReplaceAll(m[1], "&amp;", "&"),
			Label: strings.TrimSpace(m[2]),
		})
	}
	return subs, nil
}

// DownloadSubscriptionYAML downloads the YAML content from a subscription URL.
func DownloadSubscriptionYAML(subURL string) ([]byte, error) {
	resp, err := http.Get(subURL)
	if err != nil {
		return nil, fmt.Errorf("download subscription: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download subscription: status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read subscription body: %w", err)
	}
	return data, nil
}

// ClaimFreeTrial claims a one-day free trial.
// lineID: "1" for 王者套餐试用, "11" for 青铜套餐试用.
func (c *Client) ClaimFreeTrial(lineID string) error {
	params, err := c.parseTrialParams()
	if err != nil {
		return err
	}

	form := url.Values{
		"sid":      {params.SID},
		"checksum": {params.Checksum},
		"line_id":  {lineID},
		"quantity": {"1"},
	}

	resp, err := c.HTTPClient.Post(
		c.BaseURL+"/orders/request_day_trial",
		"application/x-www-form-urlencoded",
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return fmt.Errorf("post claim trial: %w", err)
	}
	defer resp.Body.Close()

	// 跟随重定向后，检查最终 URL（服务端通过 URL path 传递结果信息）
	finalURL := resp.Request.URL.String()
	decodedURL, _ := url.PathUnescape(finalURL)

	if strings.Contains(decodedURL, "success") || strings.Contains(decodedURL, "领取成功") {
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read claim response: %w", err)
	}
	bodyStr := string(body)

	if strings.Contains(bodyStr, "领取成功") {
		return nil
	}

	// 从 URL 或 body 提取中文错误信息
	if strings.Contains(decodedURL, "已申请试用") || strings.Contains(bodyStr, "已申请试用") {
		return fmt.Errorf("already claimed: trial already requested recently")
	}

	if strings.Contains(decodedURL, "error") {
		// URL 格式: /infos/show/{title}/{message}/error
		parts := strings.Split(strings.TrimSuffix(decodedURL, "/error"), "/")
		if len(parts) >= 2 {
			return fmt.Errorf("claim failed: %s", parts[len(parts)-1])
		}
	}

	return fmt.Errorf("claim failed: unexpected response (status=%d)", resp.StatusCode)
}

// Get sends a GET request using the authenticated session.
func (c *Client) Get(path string) (*http.Response, error) {
	u := c.BaseURL + path
	return c.HTTPClient.Get(u)
}

// PostForm sends a POST form request using the authenticated session.
func (c *Client) PostForm(path string, data url.Values) (*http.Response, error) {
	u := c.BaseURL + path
	return c.HTTPClient.PostForm(u, data)
}
