import bcrypt from 'bcryptjs';
import jwt from 'jsonwebtoken';
import env from '../config/env.js';
import User from '../models/user.model.js';
import ApiError from '../utils/ApiError.js';

function signToken(user) { return jwt.sign({ sub: user._id.toString(), role: user.role, name: user.name, email: user.email }, env.jwtSecret, { expiresIn: env.jwtExpiresIn }); }

export async function registerUser({ name, email, password }) {
	const normalizedEmail = email.toLowerCase().trim();
	if (await User.exists({ email: normalizedEmail })) throw new ApiError(409, 'Email đã được sử dụng');
	const user = await User.create({ name, email: normalizedEmail, passwordHash: await bcrypt.hash(password, 12) });
	return { token: signToken(user), user: { id: user._id, name: user.name, email: user.email, role: user.role } };
}

export async function loginUser({ email, password }) {
	const user = await User.findOne({ email: email.toLowerCase().trim() }).select('+passwordHash');
	if (!user || !(await bcrypt.compare(password, user.passwordHash))) throw new ApiError(401, 'Email hoặc mật khẩu không đúng');
	user.lastLoginAt = new Date();
	await user.save();
	return { token: signToken(user), user: { id: user._id, name: user.name, email: user.email, role: user.role } };
}
