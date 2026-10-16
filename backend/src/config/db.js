import mongoose from 'mongoose';
import env from './env.js';

export async function connectDatabase() {
	await mongoose.connect(env.databaseUrl, { serverSelectionTimeoutMS: 5000 });
	console.log('MongoDB connected');
}
