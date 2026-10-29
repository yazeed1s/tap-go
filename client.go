package tap

import (
	"net/http"
	"net/url"
	"strings"
)

const (
	defaultBaseURL   = "https://api.tap.company"
	defaultUserAgent = "tap-go"
)

// Client is a Tap Payments API client.
//
// The client authenticates requests with the secret key as a Bearer token,
// as required by Tap. Use a sk_test_ key while integrating.
type Client struct {
	Services

	apiKey     string
	baseURL    *url.URL
	httpClient *http.Client
	userAgent  string
	language   string
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient sets the HTTP client used for requests.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

// WithBaseURL sets the API base URL. The default is https://api.tap.company.
// Request paths include the API version, e.g. /v2/charges.
func WithBaseURL(rawURL string) Option {
	return func(c *Client) {
		if u := mustParseBaseURL(rawURL); u != nil {
			c.baseURL = u
		}
	}
}

// WithUserAgent sets the User-Agent header.
func WithUserAgent(userAgent string) Option {
	return func(c *Client) {
		if userAgent != "" {
			c.userAgent = userAgent
		}
	}
}

// WithLanguage sets Tap's lang_code header, which selects the language of
// hosted pages such as the charge redirect page. Tap accepts "en" and "ar".
func WithLanguage(langCode string) Option {
	return func(c *Client) {
		c.language = langCode
	}
}

// NewClient creates a Tap API client authenticated with secretKey.
func NewClient(secretKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:     secretKey,
		baseURL:    mustParseBaseURL(defaultBaseURL),
		httpClient: http.DefaultClient,
		userAgent:  defaultUserAgent,
	}
	for _, opt := range opts {
		opt(c)
	}
	c.Services.init(c)
	return c
}

func mustParseBaseURL(rawURL string) *url.URL {
	if rawURL == "" {
		return nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		panic(err)
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return u
}
