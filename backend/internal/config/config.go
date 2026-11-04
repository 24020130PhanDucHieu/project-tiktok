// Package config loads application settings from environment variables.
package config

import (
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
}

// IsProduction reports whether the API runs in production mode.
func (c Config) IsProduction() bool { return c.Env == "production" }

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

		JWTSecret:    get("JWT_SECRET", "development-only-change-this-secret"),
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
		ZaloBridgeSecret:      get("ZALO_BRIDGE_SECRET", "development-bridge-secret"),

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
