import mongoose from 'mongoose';

const userSchema = new mongoose.Schema({
	name: { type: String, required: true, trim: true, maxlength: 120 },
	email: { type: String, required: true, unique: true, lowercase: true, trim: true, index: true },
	passwordHash: { type: String, required: true, select: false },
	role: { type: String, enum: ['admin', 'member'], default: 'member' },
	lastLoginAt: { type: Date },
}, { timestamps: true, versionKey: false });

export default mongoose.model('User', userSchema);
