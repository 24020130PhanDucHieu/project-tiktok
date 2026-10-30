import mongoose from 'mongoose';
import env from './env.js';

export async function connectDatabase() {
	await mongoose.connect(env.databaseUrl, {
		serverSelectionTimeoutMS: 5000,
		maxPoolSize: 20,
		minPoolSize: 2,
		socketTimeoutMS: 45000,
	});
	console.log('MongoDB connected');
}
