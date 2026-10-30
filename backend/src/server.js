import app from './app.js';
import { connectDatabase } from './config/db.js';
import env from './config/env.js';
import { sendKocEmail } from './services/email.service.js';
import { buildMorningDigest, runStage2Sweep } from './services/outreach.service.js';
import { initializeZaloPersonal } from './services/zalo-personal.service.js';

let lastMorningDigestDate = '';

try {
	await connectDatabase();
	await initializeZaloPersonal();
	app.listen(env.port, () => {
		console.log(`API listening on http://127.0.0.1:${env.port}`);

		// Mục #8, #9, #10, #11-15: Quét tự động định kỳ 4 giờ/lần
		setInterval(
			async () => {
				try {
					await runStage2Sweep({ simulateNextStep: false });
				} catch (err) {
					console.error('Background sweep error:', err.message);
				}
			},
			4 * 60 * 60 * 1000,
		);

		// Mục #16: Đúng 08:00 mỗi sáng (giờ Việt Nam) gửi bản tóm tắt việc cần làm hôm nay, KOC sắp quá hạn, STALLED mới
		setInterval(
			async () => {
				try {
					const now = new Date();
					const vnHour = Number(
						now.toLocaleTimeString('en-GB', {
							timeZone: 'Asia/Ho_Chi_Minh',
							hour12: false,
							hour: '2-digit',
						}),
					);
					const todayKey = now.toISOString().slice(0, 10);
					if (vnHour === 8 && lastMorningDigestDate !== todayKey) {
						lastMorningDigestDate = todayKey;
						const digest = await buildMorningDigest();
						if (env.smtpUser) {
							await sendKocEmail({
								to: env.smtpUser,
								displayName: 'Booker & Chủ shop',
								tiktokId: 'tiktoday',
								text: digest.text,
							});
						}
					}
				} catch {
					// Bỏ qua nếu chưa cấu hình SMTP
				}
			},
			15 * 60 * 1000,
		);
	});
} catch (error) {
	console.error('Unable to start API', error);
	process.exitCode = 1;
}
