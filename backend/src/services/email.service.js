import nodemailer from 'nodemailer';
import env from '../config/env.js';

function getTransporter() {
	if (!env.smtpHost || !env.smtpUser || !env.smtpPassword) return null;
	return nodemailer.createTransport({ host: env.smtpHost, port: env.smtpPort, secure: env.smtpPort === 465, auth: { user: env.smtpUser, pass: env.smtpPassword } });
}

export async function sendKocEmail({ to, displayName, tiktokId, text }) {
	const transporter = getTransporter();
	if (!transporter) throw new Error('Email chưa được cấu hình SMTP trên server');
	return transporter.sendMail({ from: env.mailFrom, to, subject: `Cơ hội hợp tác TikTok dành cho ${displayName}`, text: text || `Chào ${displayName},\n\nMình là Tiktoday. Mình thấy kênh TikTok @${tiktokId} phù hợp với một chiến dịch bên mình. Bạn cho mình xin phép trao đổi brief và mức phí hợp tác nhé?\n\nTrân trọng,\nTiktoday` });
}