import app from './app.js';
import { connectDatabase } from './config/db.js';
import env from './config/env.js';
import { runStage2Sweep } from './services/outreach.service.js';
import { initializeZaloPersonal } from './services/zalo-personal.service.js';

try {
	await connectDatabase();
	await initializeZaloPersonal();
	app.listen(env.port, () => {
		console.log(`API listening on http://127.0.0.1:${env.port}`);

		// Định kỳ 4 giờ/lần: Quét vận đơn J&T Express (D3), nhắc lịch Zalo/Gmail (C3) và nhắc lên video 5N/8N/12N (E1-E7)
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
	});
} catch (error) {
	console.error('Unable to start API', error);
	process.exitCode = 1;
}
