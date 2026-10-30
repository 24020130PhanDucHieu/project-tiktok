import assert from 'node:assert/strict';
import test from 'node:test';
import { queryJtTracking } from '../src/services/jt.service.js';
import {
	buildInviteMessage,
	canSendProactiveMessage,
	isWithinAllowedSendWindow,
	parseKocInboundText,
} from '../src/services/outreach.service.js';
import {
	extractContactFromBio,
	fetchRealTikTokChannel,
	fetchRealTikTokVideoMetrics,
} from '../src/services/tiktok.service.js';

test('1. [API THẬT · TIKTOK] Lấy thông tin kênh thật, follower, avatar và bóc tách SĐT/Email bio từ TikTok', async () => {
	const result = await fetchRealTikTokChannel('trlyng.review');

	assert.equal(result.found, true, 'Phải tìm thấy kênh TikTok thật @trlyng.review');
	assert.equal(result.profile.tiktokId, 'trlyng.review');
	assert.equal(result.profile.displayName, 'Tủ đồ da ngăm', 'Phải lấy đúng tên hiển thị thật của kênh');
	assert.ok(result.profile.followers > 1000, `Số follower thật phải > 1000 (nhận được: ${result.profile.followers})`);
	assert.ok(result.profile.avatarUrl.startsWith('http'), 'Phải lấy được URL avatar thật từ TikTok CDN');

	const bioContact = extractContactFromBio(
		'Liên hệ cv: 0368303121\ntrlyng2@gmail.com\n1m63 50kg undertone trung tính',
	);
	assert.equal(bioContact.phone, '0368303121', 'Phải tự động bóc tách được SĐT thật trong tiểu sử (Bio)');
	assert.equal(bioContact.email, 'trlyng2@gmail.com', 'Phải tự động bóc tách được Email thật trong tiểu sử (Bio)');
	assert.ok(Array.isArray(result.videos), 'Danh sách video trả về phải là mảng hợp lệ');
});

test('2. [API THẬT · TIKTOK OEMBED] Lấy metadata video thật qua cổng TikTok Official oEmbed API', async () => {
	const videoUrl = 'https://www.tiktok.com/@trlyng.review/video/7619258382867811601';
	const meta = await fetchRealTikTokVideoMetrics(videoUrl);

	assert.ok(meta.title.length > 0, 'Phải lấy được tiêu đề video từ oEmbed API');
	assert.ok(meta.postedAt instanceof Date, 'Phải ghi nhận thời điểm đăng');
});

test('3. [API THẬT · J&T EXPRESS] Tra cứu hành trình vận đơn trực tiếp từ J&T Express Việt Nam', async () => {
	const billCode = '841000098765';
	const tracking = await queryJtTracking(billCode);

	assert.equal(tracking.billCode, billCode);
	assert.equal(typeof tracking.delivered, 'boolean');
	assert.ok(
		['jt-openapi', 'jtexpress.vn', 'jt-tracker'].includes(tracking.source),
		`Nguồn tra cứu J&T phải hợp lệ (nhận được: ${tracking.source})`,
	);
	assert.ok(tracking.statusText.length > 0, 'Phải trả về mô tả trạng thái vận đơn J&T');
});

test('4. [CHATBOT · MỤC #7, #9, #13, #15] Phân loại chính xác các ý định của KOC và bóc tách SĐT + Địa chỉ', () => {
	// #7: KOC gửi địa chỉ + SĐT
	const addrParsed = parseKocInboundText(
		'Địa chỉ nhận mẫu: Ký túc xá Ngoại ngữ VNU, Cầu Giấy, Hà Nội - SĐT 0988123456',
	);
	assert.equal(addrParsed.intent, 'PROVIDED_ADDRESS');
	assert.equal(addrParsed.phone, '0988123456');
	assert.ok(addrParsed.shippingAddress.includes('Ký túc xá Ngoại ngữ VNU'));

	// #7: KOC đồng ý nhưng chưa gửi địa chỉ
	const agreeParsed = parseKocInboundText('Ok shop, mình đồng ý nhận mẫu nhé!');
	assert.equal(agreeParsed.intent, 'AGREE_NO_ADDRESS');

	// Cấm kỵ #1: KOC đòi giá cast ngoài bảng đã duyệt -> Chuyển sang NEGOTIATE_CAST để chờ chủ shop duyệt
	const castParsed = parseKocInboundText('Giá cast của mình là 1 triệu và tăng hoa hồng lên 20% mới nhận nhé');
	assert.equal(castParsed.intent, 'NEGOTIATE_CAST');
	assert.equal(castParsed.suggestedDeclineCode, 'R02');

	// #9: KOC xác nhận đã nhận hàng tự vận chuyển ("rồi")
	const receivedParsed = parseKocInboundText('Mình nhận được hàng rồi shop ơi');
	assert.equal(receivedParsed.intent, 'CONFIRMED_RECEIVED');

	// #13: KOC gửi link video TikTok
	const videoParsed = parseKocInboundText(
		'Mình gửi shop link video nhé: https://www.tiktok.com/@trlyng.review/video/7619258382867811601',
	);
	assert.equal(videoParsed.intent, 'SUBMITTED_VIDEO');
	assert.equal(videoParsed.videoUrl, 'https://www.tiktok.com/@trlyng.review/video/7619258382867811601');

	// #15: KOC báo không hợp sản phẩm
	const notFitParsed = parseKocInboundText('Sản phẩm này không hợp sản phẩm với mình, cho mình xin gửi trả lại');
	assert.equal(notFitParsed.intent, 'NOT_FIT_RETURN');

	// Đề xuất mã lý do từ chối khi KOC bận lịch (R04)
	const declineParsed = parseKocInboundText('Đợt này mình bận lịch quá xin phép từ chối nhé');
	assert.equal(declineParsed.intent, 'DECLINED');
	assert.equal(declineParsed.suggestedDeclineCode, 'R04');
});

test('5. [CHỐNG SPAM & ĐIỀU CẤM KỴ CHATBOT] Khung giờ 08:30–20:30, chặn DNC, chặn 24h, không viết "tặng" hay "dùng thật"', () => {
	// Kiểm tra khung giờ 10:00 sáng VN (03:00 UTC) -> Cho phép
	const morningVn = new Date('2026-10-30T03:00:00Z');
	assert.equal(isWithinAllowedSendWindow(morningVn), true);

	// Kiểm tra khung giờ 23:00 đêm VN (16:00 UTC) -> Chặn
	const lateNightVn = new Date('2026-10-30T16:00:00Z');
	assert.equal(isWithinAllowedSendWindow(lateNightVn), false);

	// Chặn gửi khi KOC đặt cờ dnc = true
	assert.equal(canSendProactiveMessage({ tiktokId: 'koc1', dnc: true }, { bypassWindow: true }).allowed, false);

	// Chặn gửi khi KOC đã nhận 1 tin chủ động trong vòng 24 giờ
	const twoHoursAgo = new Date(Date.now() - 2 * 60 * 60 * 1000);
	const recentCheck = canSendProactiveMessage(
		{ tiktokId: 'koc2', dnc: false, daHopTac: false, lastContactedAt: twoHoursAgo },
		{ bypassWindow: false },
	);
	assert.equal(recentCheck.allowed, false);

	// Kiểm tra nội dung tin mời: Không bao giờ chứa từ "tặng" hay "dùng thật"
	const inviteText = buildInviteMessage({
		tiktokId: 'trlyng.review',
		displayName: 'Tủ đồ da ngăm',
		productName: 'Khăn lạnh thể thao Cooltech',
		castFee: 0,
		commissionRate: 10,
	});
	assert.equal(/tặng/i.test(inviteText), false, 'Tin nhắn mời tuyệt đối không được viết chữ "tặng" sản phẩm');
	assert.equal(/dùng thật/i.test(inviteText), false, 'Tin nhắn mời tuyệt đối không được khen kiểu "dùng thật"');
	assert.equal(
		/trải nghiệm/i.test(inviteText),
		true,
		'Tin nhắn mời phải ghi rõ chính sách gửi sản phẩm mẫu để trải nghiệm',
	);
});
