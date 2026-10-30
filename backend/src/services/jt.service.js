import crypto from 'node:crypto';
import env from '../config/env.js';

function createJtDigest(bizContentString, privateKey) {
	const md5Hex = crypto.createHash('md5').update(bizContentString + privateKey, 'utf8').digest('hex');
	return Buffer.from(md5Hex, 'utf8').toString('base64');
}

/**
 * Tra cứu hành trình vận đơn thực tế từ J&T Express Việt Nam:
 * 1. Nếu đã cấu hình JT_API_ACCOUNT & JT_PRIVATE_KEY: Gọi thẳng J&T Open Platform API (/trace/trace).
 * 2. Nếu chưa cấu hình tài khoản doanh nghiệp: Gọi trực tiếp trang tra cứu công khai https://jtexpress.vn/vi/tracking
 */
export async function queryJtTracking(billCode) {
	const cleanCode = String(billCode || '').trim();
	if (!cleanCode) {
		throw new Error('Thiếu mã vận đơn J&T Express');
	}

	// 1. Gọi J&T Open Platform API chính thức nếu có cấu hình Merchant
	if (env.jtApiAccount && env.jtPrivateKey) {
		const bizContent = JSON.stringify({ billCodes: cleanCode });
		const digest = createJtDigest(bizContent, env.jtPrivateKey);
		const body = new URLSearchParams({ bizContent });

		const response = await fetch(`${env.jtApiUrl}/trace/trace`, {
			method: 'POST',
			headers: {
				'Content-Type': 'application/x-www-form-urlencoded',
				apiAccount: env.jtApiAccount,
				digest,
				timestamp: String(Date.now()),
			},
			body: body.toString(),
			signal: AbortSignal.timeout(10000),
		});

		const json = await response.json();
		const details = json?.data?.[0]?.details || [];
		const latest = details[0] || null;
		const scanType = (latest?.scanType || latest?.desc || '').toLowerCase();
		const delivered =
			scanType.includes('ký nhận') ||
			scanType.includes('giao hàng thành công') ||
			scanType.includes('delivered') ||
			scanType.includes('sign');

		return {
			source: 'jt-openapi',
			billCode: cleanCode,
			delivered,
			statusText: latest?.desc || (delivered ? 'Đã giao hàng thành công (J&T OpenAPI)' : 'Đang luân chuyển tại bưu cục J&T'),
			details,
		};
	}

	// 2. Tra cứu trực tiếp từ cổng công khai jtexpress.vn
	try {
		const publicUrl = `https://jtexpress.vn/vi/tracking?type=track&billcode=${encodeURIComponent(cleanCode)}`;
		const response = await fetch(publicUrl, {
			headers: {
				'User-Agent':
					'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36',
				Accept: 'text/html,application/xhtml+xml',
			},
			signal: AbortSignal.timeout(9000),
		});

		if (response.ok) {
			const html = await response.text();
			const lower = html.toLowerCase();
			const delivered =
				lower.includes('giao hàng thành công') ||
				lower.includes('đã ký nhận') ||
				lower.includes('ký nhận thành công');
			const inTransit =
				lower.includes('đang vận chuyển') ||
				lower.includes('đã đến bưu cục') ||
				lower.includes('đang giao hàng') ||
				lower.includes('nhận kiện');

			let statusText = 'Đã ghi nhận mã vận đơn trên cổng J&T Express';
			if (delivered) statusText = 'J&T Express: Giao hàng thành công (Đã ký nhận)';
			else if (inTransit) statusText = 'J&T Express: Đang luân chuyển giữa các bưu cục';

			return {
				source: 'jtexpress.vn',
				billCode: cleanCode,
				delivered,
				statusText,
			};
		}
	} catch {
		// Nếu mạng bị chặn hoặc timeout, trả về trạng thái đang theo dõi
	}

	return {
		source: 'jt-tracker',
		billCode: cleanCode,
		delivered: false,
		statusText: `Đang theo dõi vận đơn J&T #${cleanCode} (Quét tự động 4 giờ/lần)`,
	};
}
