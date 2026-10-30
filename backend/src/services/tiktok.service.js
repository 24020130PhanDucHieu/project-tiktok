import env from '../config/env.js';

function formatCompact(num) {
	const n = Number(num) || 0;
	if (n >= 1000000) return `${(n / 1000000).toFixed(1).replace('.0', '')}M`;
	if (n >= 1000) return `${(n / 1000).toFixed(1).replace('.0', '')}k`;
	return String(n);
}

function getDateFromVideoId(videoId) {
	try {
		const tsSeconds = Number(BigInt(videoId) >> 32n);
		if (tsSeconds > 1500000000 && tsSeconds < 2500000000) {
			return new Date(tsSeconds * 1000).toLocaleDateString('vi-VN', {
				day: '2-digit',
				month: '2-digit',
			});
		}
	} catch {
		// Ignore BigInt parse failure
	}
	return new Date().toLocaleDateString('vi-VN', { day: '2-digit', month: '2-digit' });
}

function parseMetaCount(str = '') {
	const clean = String(str).trim().toLowerCase();
	const val = parseFloat(clean);
	if (Number.isNaN(val)) return 0;
	if (clean.endsWith('b')) return Math.round(val * 1000000000);
	if (clean.endsWith('m')) return Math.round(val * 1000000);
	if (clean.endsWith('k')) return Math.round(val * 1000);
	return Math.round(val);
}

function extractContactFromBio(signature = '') {
	const text = String(signature || '');
	const phoneMatch = text.match(/(?:\+84|0)[35789]\d{8}/);
	const emailMatch = text.match(/[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}/);
	return {
		phone: phoneMatch ? phoneMatch[0].replace(/^\+84/, '0') : '',
		email: emailMatch ? emailMatch[0].toLowerCase() : '',
	};
}

/**
 * Lấy dữ liệu kênh TikTok thực tế trực tiếp từ máy chủ TikTok:
 * 1. Gọi https://www.tiktok.com/embed/@uniqueId (__FRONTITY_CONNECT_STATE__) để lấy userInfo + videoList thực tế.
 * 2. Fallback sang OpenGraph Meta Tags của TikTok hoặc RapidAPI nếu có cấu hình.
 */
export { extractContactFromBio };

export async function fetchRealTikTokChannel(rawHandle) {
	const uniqueId = String(rawHandle || '').trim().replace(/^@/, '').toLowerCase();
	if (!uniqueId) throw new Error('Thiếu TikTok ID');

	let profile = null;
	let videos = [];

	// 1. Lấy trực tiếp từ TikTok Embed Endpoint (__FRONTITY_CONNECT_STATE__) với tự động retry nếu gặp overload-protect
	for (let attempt = 0; attempt < 5; attempt += 1) {
		try {
			const embedRes = await fetch(`https://www.tiktok.com/embed/@${encodeURIComponent(uniqueId)}`, {
				signal: AbortSignal.timeout(10000),
			});

			if (embedRes.ok) {
				const html = await embedRes.text();
				const markerIdx = html.indexOf('FRONTITY_CONNECT_STATE');
				if (markerIdx !== -1) {
					const startJson = html.indexOf('>', markerIdx) + 1;
					const endJson = html.indexOf('</script>', startJson);
					if (startJson > 0 && endJson > startJson) {
						const parsed = JSON.parse(html.slice(startJson, endJson));
						const dataMap = parsed?.source?.data || {};
						const entry = Object.values(dataMap)[0];
						const u = entry?.userInfo;
						if (u && u.uniqueId) {
							const bioContact = extractContactFromBio(u.signature);
							profile = {
								tiktokId: u.uniqueId || uniqueId,
								displayName: u.nickname || u.uniqueId || uniqueId,
								avatarUrl: u.avatarThumbUrl || '',
								followers: Math.max(0, Number(u.followerCount) || 0),
								heartCount: Math.max(0, Number(u.heartCount) || 0),
								videoCount: (entry?.videoList || []).length,
								signature: u.signature || '',
								phone: bioContact.phone,
								email: bioContact.email,
							};

							videos = (entry?.videoList || []).slice(0, 15).map((v) => ({
								id: String(v.id || ''),
								date: getDateFromVideoId(v.id),
								title: v.desc || 'Video TikTok',
								url: `https://www.tiktok.com/@${u.uniqueId || uniqueId}/video/${v.id}`,
								coverUrl: v.coverUrl || v.originCoverUrl || '',
								views: formatCompact(v.playCount),
								likes: '—',
								comments: '—',
								shares: '—',
								product: v.desc?.includes('#')
									? v.desc
											.split('#')
											.slice(1, 3)
											.map((t) => `#${t.trim()}`)
											.join(' ')
									: 'Video trên kênh',
							}));
							break;
						}
					}
				}
			}
			await new Promise((r) => setTimeout(r, 450 * (attempt + 1)));
		} catch {
			// Thử lại hoặc chuyển sang OpenGraph Crawler
		}
	}

	// 2. Bổ sung Display Name chuẩn (ví dụ "Tủ đồ da ngăm") và Avatar HD từ OpenGraph Meta của TikTok
	try {
		const metaRes = await fetch(`https://www.tiktok.com/@${encodeURIComponent(uniqueId)}`, {
			headers: { 'User-Agent': 'facebookexternalhit/1.1' },
			signal: AbortSignal.timeout(8000),
		});
		if (metaRes.ok) {
			const html = await metaRes.text();
			const titleMatch = html.match(/<meta property="og:title" content="([^"]+)"/i);
			const descMatch = html.match(/<meta property="og:description" content="([^"]+)"/i);
			const imgMatch = html.match(/<meta property="og:image" content="([^"]+)"/i);

			if (descMatch?.[1]) {
				const desc = descMatch[1];
				const flwMatch = desc.match(/([\d.]+[kmb]?)\s+Followers/i);
				const rawName = (titleMatch?.[1] || uniqueId).replace(/\s+on TikTok$/i, '').trim();
				const hdAvatar = imgMatch?.[1]?.replace(/&amp;/g, '&') || '';
				const parsedFlw = flwMatch ? parseMetaCount(flwMatch[1]) : 0;
				const bioContact = extractContactFromBio(desc);

				if (!profile) {
					profile = {
						tiktokId: uniqueId,
						displayName: rawName,
						avatarUrl: hdAvatar,
						followers: parsedFlw,
						heartCount: 0,
						videoCount: videos.length,
						signature: desc,
						phone: bioContact.phone,
						email: bioContact.email,
					};
				} else {
					if (rawName && rawName !== uniqueId) profile.displayName = rawName;
					if (hdAvatar) profile.avatarUrl = hdAvatar;
					if (!profile.followers && parsedFlw) profile.followers = parsedFlw;
					if (!profile.phone && bioContact.phone) profile.phone = bioContact.phone;
					if (!profile.email && bioContact.email) profile.email = bioContact.email;
				}
			}
		}
	} catch {
		// Ignore
	}

	return {
		found: Boolean(profile),
		profile: profile || {
			tiktokId: uniqueId,
			displayName: uniqueId,
			avatarUrl: '',
			followers: 0,
			heartCount: 0,
			videoCount: 0,
		},
		videos,
	};
}

/**
 * Tra cứu thông tin thực tế của 1 Video TikTok từ URL qua TikTok Official oEmbed API
 */
export async function fetchRealTikTokVideoMetrics(videoUrl) {
	const cleanUrl = String(videoUrl || '').trim();
	if (!cleanUrl) throw new Error('Thiếu đường dẫn video TikTok');

	try {
		const oembedRes = await fetch(`https://www.tiktok.com/oembed?url=${encodeURIComponent(cleanUrl)}`, {
			headers: {
				'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/124.0.0.0',
			},
			signal: AbortSignal.timeout(8000),
		});
		if (oembedRes.ok) {
			const oembed = await oembedRes.json();
			return {
				videoId: String(oembed.embed_product_id || ''),
				title: oembed.title || 'Video TikTok',
				authorName: oembed.author_name || '',
				authorUniqueId: oembed.author_unique_id || '',
				coverUrl: oembed.thumbnail_url || '',
				postedAt: new Date(),
			};
		}
	} catch {
		// Ignore
	}

	return {
		videoId: '',
		title: 'Video hợp tác KOC',
		coverUrl: '',
		postedAt: new Date(),
	};
}
