import 'dotenv/config';

const env = {
	port: Number(process.env.PORT || 5000),
	nodeEnv: process.env.NODE_ENV || 'development',
	databaseUrl: process.env.MONGODB_URI || 'mongodb://127.0.0.1:27017/tiktoday_booking',
	frontendUrl: process.env.FRONTEND_URL || 'http://127.0.0.1:5173',
	jwtSecret: process.env.JWT_SECRET || 'development-only-change-this-secret',
	jwtExpiresIn: process.env.JWT_EXPIRES_IN || '1d',
	zaloOaAccessToken: process.env.ZALO_OA_ACCESS_TOKEN || '',
	zaloPersonalCookie: process.env.ZALO_PERSONAL_COOKIE || '',
	zaloPersonalImei: process.env.ZALO_PERSONAL_IMEI || '',
	zaloPersonalUserAgent: process.env.ZALO_PERSONAL_USER_AGENT || '',
	smtpHost: process.env.SMTP_HOST || '',
	smtpPort: Number(process.env.SMTP_PORT || 587),
	smtpUser: process.env.SMTP_USER || '',
	smtpPassword: process.env.SMTP_PASSWORD || '',
	mailFrom: process.env.MAIL_FROM || process.env.SMTP_USER || '',
	// J&T Express Open Platform API
	jtApiUrl: process.env.JT_API_URL || 'https://openapi.jtexpress.vn/webopenplatformapi/api',
	jtApiAccount: process.env.JT_API_ACCOUNT || '',
	jtPrivateKey: process.env.JT_PRIVATE_KEY || '',
	jtCustomerCode: process.env.JT_CUSTOMER_CODE || '',
	// TikTok Data / RapidAPI (Tùy chọn bổ sung ngoài public scraping)
	tiktokRapidApiKey: process.env.TIKTOK_RAPIDAPI_KEY || '',
	tiktokRapidApiHost: process.env.TIKTOK_RAPIDAPI_HOST || 'tiktok-scraper7.p.rapidapi.com',
};

export default env;
