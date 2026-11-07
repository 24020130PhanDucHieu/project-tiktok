// Package config loads application settings from environment variables.
package config

import (
	"errors"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config holds every runtime setting of the API.
type Config struct {
	Port        string
	Env         string
	MongoURI    string
	MongoDBName string
	FrontendURL string

	JWTSecret    string
	JWTExpiresIn time.Duration

	// Account policy
	AllowRegistration bool   // public self-registration (off by default; admins create accounts)
	AdminEmail        string // bootstrap admin created on first start when no admin exists
	AdminPassword     string
	PasswordResetTTL  time.Duration

	ZaloOAAccessToken     string
	ZaloPersonalCookie    string
	ZaloPersonalIMEI      string
	ZaloPersonalUserAgent string
	ZaloBridgeURL         string
	ZaloBridgeSecret      string
	// ZaloWebhookSecret guards the Zalo OA webhook (POST /webhooks/zalo). Empty = the webhook is not served at all.
	ZaloWebhookSecret string

	SMTPHost     string
	SMTPPort     int
	SMTPUser     string
	SMTPPassword string
	MailFrom     string

	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string

	JTAPIURL       string
	JTAPIAccount   string
	JTPrivateKey   string
	JTCustomerCode string

	TikTokRapidAPIKey  string
	TikTokRapidAPIHost string

	// TikTok Shop Open Platform (read-only integration, see service/ttshop.go)
	TTSAppKey      string
	TTSAppSecret   string
	TTSEnv         string
	TTSAPIBaseURL  string
	TTSAuthBaseURL string
	TTSLogFile     string // append-only log of every TikTok Shop call (empty = memory + stdout only)

	// Target collaboration: the on/off switch for sending is in the app (database, off by default); this is the cap.
	TTSTargetInviteMaxCreators int
}

// IsProduction reports whether the API runs in production mode.
func (c Config) IsProduction() bool { return c.Env == "production" }

// The development defaults: anyone can read them in the source, so production must never run with them.
const (
	defaultJWTSecret    = "development-only-change-this-secret"
	defaultBridgeSecret = "development-bridge-secret"
	minSecretLength     = 32
	// examplePlaceholderPrefix starts every secret placeholder of .env.example ("replace-with-a-long-random-secret").
	examplePlaceholderPrefix = "replace-with"
)

// Validate refuses a production start whose secrets are still the development defaults or too short. A forged
// JWT_SECRET lets anybody sign in as an admin, and the bridge secret lets anybody send Zalo messages.
func (c Config) Validate() error {
	if !c.IsProduction() {
		return nil
	}
	var problems []string
	check := func(name, value, dev string) {
		switch {
		case value == dev:
			problems = append(problems, name+" is still the development default")
		case strings.HasPrefix(strings.ToLower(value), examplePlaceholderPrefix):
			problems = append(problems, name+" is still the placeholder of .env.example")
		case len(value) < minSecretLength:
			problems = append(problems, name+" is shorter than "+strconv.Itoa(minSecretLength)+" characters")
		}
	}
	check("JWT_SECRET", c.JWTSecret, defaultJWTSecret)
	check("ZALO_BRIDGE_SECRET", c.ZaloBridgeSecret, defaultBridgeSecret)
	if len(problems) > 0 {
		return errors.New("unsafe production configuration: " + strings.Join(problems, "; "))
	}
	return nil
}

// Load reads .env (if present) and the process environment.
func Load() Config {
	_ = godotenv.Load()
	smtpUser := get("SMTP_USER", "")
	return Config{
		Port:        get("PORT", "5000"),
		Env:         get("APP_ENV", get("NODE_ENV", "development")),
		MongoURI:    get("MONGODB_URI", "mongodb://127.0.0.1:27017/tiktoday_booking"),
		MongoDBName: get("MONGODB_DB", dbNameFromURI(get("MONGODB_URI", ""))),
		FrontendURL: get("FRONTEND_URL", "http://127.0.0.1:5173"),

		JWTSecret:    get("JWT_SECRET", defaultJWTSecret),
		JWTExpiresIn: parseDuration(get("JWT_EXPIRES_IN", "1d")),

		AllowRegistration: get("ALLOW_REGISTRATION", "false") == "true",
		AdminEmail:        strings.ToLower(get("ADMIN_EMAIL", "")),
		AdminPassword:     get("ADMIN_PASSWORD", ""),
		PasswordResetTTL:  parseDuration(get("PASSWORD_RESET_TTL", "10m")),

		ZaloOAAccessToken:     get("ZALO_OA_ACCESS_TOKEN", ""),
		ZaloPersonalCookie:    get("ZALO_PERSONAL_COOKIE", ""),
		ZaloPersonalIMEI:      get("ZALO_PERSONAL_IMEI", ""),
		ZaloPersonalUserAgent: get("ZALO_PERSONAL_USER_AGENT", ""),
		ZaloBridgeURL:         get("ZALO_BRIDGE_URL", "http://127.0.0.1:5100"),
		ZaloBridgeSecret:      get("ZALO_BRIDGE_SECRET", defaultBridgeSecret),
		ZaloWebhookSecret:     get("ZALO_WEBHOOK_SECRET", ""),

		SMTPHost:     get("SMTP_HOST", ""),
		SMTPPort:     getInt("SMTP_PORT", 587),
		SMTPUser:     smtpUser,
		SMTPPassword: get("SMTP_PASSWORD", ""),
		MailFrom:     get("MAIL_FROM", smtpUser),

		GoogleClientID:     get("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: get("GOOGLE_CLIENT_SECRET", ""),
		GoogleRedirectURL:  get("GOOGLE_REDIRECT_URL", "http://127.0.0.1:5000/api/v1/gmail/callback"),

		JTAPIURL:       get("JT_API_URL", "https://openapi.jtexpress.vn/webopenplatformapi/api"),
		JTAPIAccount:   get("JT_API_ACCOUNT", ""),
		JTPrivateKey:   get("JT_PRIVATE_KEY", ""),
		JTCustomerCode: get("JT_CUSTOMER_CODE", ""),

		TikTokRapidAPIKey:  get("TIKTOK_RAPIDAPI_KEY", ""),
		TikTokRapidAPIHost: get("TIKTOK_RAPIDAPI_HOST", "tiktok-scraper7.p.rapidapi.com"),

		TTSAppKey:      get("TTS_APP_KEY", ""),
		TTSAppSecret:   get("TTS_APP_SECRET", ""),
		TTSEnv:         get("TTS_ENV", "sandbox"),
		TTSAPIBaseURL:  strings.TrimRight(get("TTS_API_BASE_URL", "https://open-api.tiktokglobalshop.com"), "/"),
		TTSAuthBaseURL: strings.TrimRight(get("TTS_AUTH_BASE_URL", "https://auth.tiktok-shops.com"), "/"),
		TTSLogFile:     get("TTS_LOG_FILE", "logs/ttshop.log"),

		TTSTargetInviteMaxCreators: getInt("TTS_TARGET_INVITE_MAX_CREATORS", 10),
	}
}

func get(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getInt(key string, fallback int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil {
		return v
	}
	return fallback
}

// parseDuration accepts Go durations ("24h") and the jsonwebtoken style ("1d", "7d").
func parseDuration(v string) time.Duration {
	if len(v) > 1 && v[len(v)-1] == 'd' {
		if n, err := strconv.Atoi(v[:len(v)-1]); err == nil {
			return time.Duration(n) * 24 * time.Hour
		}
	}
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	return 24 * time.Hour
}

// dbNameFromURI extracts the database name from a mongodb:// URI path (default "tiktoday_booking").
func dbNameFromURI(uri string) string {
	if u, err := url.Parse(uri); err == nil {
		if name := strings.Trim(u.Path, "/"); name != "" {
			return name
		}
	}
	return "tiktoday_booking"
}
