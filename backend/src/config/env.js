import 'dotenv/config';

const env = {
	port: Number(process.env.PORT || 5000),
	nodeEnv: process.env.NODE_ENV || 'development',
	databaseUrl: process.env.MONGODB_URI || 'mongodb://127.0.0.1:27017/tiktoday_booking',
	frontendUrl: process.env.FRONTEND_URL || 'http://127.0.0.1:5173',
	jwtSecret: process.env.JWT_SECRET || 'development-only-change-this-secret',
	jwtExpiresIn: process.env.JWT_EXPIRES_IN || '1d',
};

export default env;
