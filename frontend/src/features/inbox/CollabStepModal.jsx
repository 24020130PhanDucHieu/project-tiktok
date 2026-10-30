import { useState } from 'react';
import api from '../../lib/axios';
import { DECLINE_REASONS, POST_RECEIVE_REASONS } from '../../lib/constants';
import { formatCurrency } from '../../lib/formatters';

export default function CollabStepModal({ stepKey, koc, onClose, onSave, onSyncChannel, onTrackShipment }) {
	// Step 1 & 2
	const [castFee, setCastFee] = useState(koc.castFee || 0);
	const [productName, setProductName] = useState(koc.productName || '');
	const [campaign, setCampaign] = useState(koc.campaign || 'Đợt T11');
	const [commissionRate, setCommissionRate] = useState(koc.commissionRate || 10);
	const [declineCode, setDeclineCode] = useState('');
	const [declineNote, setDeclineNote] = useState(koc.declineNote || '');

	// Step 3: Duyệt & gửi mẫu (J&T Express / Tự vận chuyển)
	const [recipient, setRecipient] = useState(koc.recipient || koc.name);
	const [phone, setPhone] = useState(koc.phone || '');
	const [shippingAddress, setShippingAddress] = useState(koc.shippingAddress || '');
	const [shippingCarrier, setShippingCarrier] = useState(koc.shippingCarrier || 'J&T Express');
	const [trackingCode, setTrackingCode] = useState(koc.trackingCode || '');
	const [shippedAt, setShippedAt] = useState(
		koc.shippedAt ? new Date(koc.shippedAt).toISOString().slice(0, 10) : new Date().toISOString().slice(0, 10),
	);

	// Step 4: KOC đăng bài / Nhắc video / Hoàn hàng P01-P03 / STALLED
	const [videoOutcome, setVideoOutcome] = useState(
		['NOT_FIT', 'RETURNING', 'RETURNED'].includes(koc.stage)
			? 'NOT_FIT'
			: koc.stage === 'STALLED'
				? 'STALLED'
				: 'POSTED',
	);
	const [videoLink, setVideoLink] = useState(koc.videoUrl || '');
	const [postReceiveCode, setPostReceiveCode] = useState(koc.postReceiveCode || 'P01');
	const [postReceiveNote, setPostReceiveNote] = useState(koc.postReceiveNote || '');
	const [returnTrackingCode, setReturnTrackingCode] = useState(koc.returnTrackingCode || '');
	const [returnCondition, setReturnCondition] = useState(koc.returnCondition || 'Nguyên vẹn');

	// Step 5: Hiệu quả (Views, Đơn hàng, GMV)
	const [videoViews, setVideoViews] = useState(koc.videoViews || 0);
	const [videoOrders, setVideoOrders] = useState(koc.videoOrders || 0);
	const [gmvGenerated, setGmvGenerated] = useState(koc.videoGmv || koc.gmv || 0);

	function handleSubmit(e) {
		e.preventDefault();
		if (stepKey === 'cast') {
			const fee = Number(castFee) || 0;
			onSave(
				{
					action: 'approve_cast',
					castFee: fee,
					collabStep: 1,
				},
				`Đã chốt phí cast (${fee > 0 ? formatCurrency(fee) : 'Freecast 0đ'}) cho ${koc.name}.`,
			);
		} else if (stepKey === 'mode') {
			if (declineCode) {
				onSave(
					{
						action: 'decline',
						declineCode,
						declineNote,
						collabStep: 1,
					},
					`Đã ghi nhận ${koc.name} từ chối hợp tác (${declineCode}).`,
				);
			} else {
				onSave(
					{
						action: 'qualify',
						productName,
						campaign,
						commissionRate: Number(commissionRate) || 10,
						castFee: Number(castFee) || 0,
						collabStep: 2,
					},
					`Đã chọn hình thức hợp tác cho ${koc.name} — Chuyển sang bước Duyệt & gửi mẫu.`,
				);
			}
		} else if (stepKey === 'sample') {
			onSave(
				{
					action: 'ship_sample',
					recipient,
					phone,
					shippingAddress,
					shippingCarrier,
					trackingCode,
					shippedAt,
					collabStep: 3,
				},
				`Đã tạo vận đơn ${shippingCarrier}${trackingCode ? ` (#${trackingCode})` : ''} giao tới ${shippingAddress}.`,
			);
		} else if (stepKey === 'video') {
			if (videoOutcome === 'POSTED') {
				api.post('/kocs/video/lookup', { videoUrl: videoLink })
					.then(({ data }) => {
						const vMeta = data?.data || {};
						onSave(
							{
								action: 'post_video',
								videoUrl: videoLink,
								videoId: vMeta.videoId || '',
								videoTitle: vMeta.title || '',
								collabStep: 4,
							},
							data?.message || `Đã xác thực và ghi nhận video TikTok của ${koc.name}.`,
						);
					})
					.catch(() => {
						onSave(
							{
								action: 'post_video',
								videoUrl: videoLink,
								collabStep: 4,
							},
							`Đã ghi nhận video TikTok của ${koc.name}.`,
						);
					});
			} else if (videoOutcome === 'NOT_FIT') {
				onSave(
					{
						action: 'return_product',
						postReceiveCode,
						postReceiveNote,
						returnTrackingCode,
						returnCondition,
						collabStep: 3,
					},
					`Đã chuyển sang theo dõi hoàn hàng (${postReceiveCode}) cho ${koc.name}.`,
				);
			} else if (videoOutcome === 'STALLED') {
				onSave(
					{
						action: 'video_reply',
						kocVideoReply: 'SILENT',
						collabStep: 3,
					},
					`Đã gắn cờ đỏ STALLED (Quá 3 lần nhắc im lặng) cho ${koc.name}.`,
				);
			}
		} else if (stepKey === 'result') {
			onSave(
				{
					action: 'complete_collab',
					videoViews: Number(videoViews) || 0,
					videoOrders: Number(videoOrders) || 0,
					videoGmv: Number(gmvGenerated) || 0,
					markDaHopTac: true,
					collabStep: 5,
				},
				`Đã hoàn tất nghiệm thu hợp tác với ${koc.name} (GMV: ${formatCurrency(gmvGenerated)}).`,
			);
		}
	}

	return (
		<div className="modal-backdrop" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
			<form className="tiktoday-modal" onSubmit={handleSubmit}>
				<div className="tm-header">
					<div>
						<h2>
							{stepKey === 'cast' && 'Bước 1 · Chốt phí cast'}
							{stepKey === 'mode' && 'Bước 2 · Chọn hình thức hợp tác'}
							{stepKey === 'sample' && 'Bước 3 · Duyệt & gửi mẫu (J&T Express)'}
							{stepKey === 'video' && 'Bước 4 · KOC đăng bài & Theo dõi sau nhận mẫu'}
							{stepKey === 'result' && 'Bước 5 · Hiệu quả hợp tác'}
						</h2>
						<p>
							KOC: <b>{koc.name}</b> ({koc.handle})
						</p>
					</div>
					<button type="button" className="tm-close" onClick={onClose}>
						×
					</button>
				</div>

				<div className="tm-body">
					{stepKey === 'cast' && (
						<>
							<label className="tm-section-label">Mức phí cast thống nhất với KOC (0 = Freecast)</label>
							<div className="tm-fee-wrap">
								<input
									type="number"
									value={castFee}
									onChange={(e) => setCastFee(e.target.value)}
									placeholder="Nhập phí cast"
								/>
								<span>đ</span>
							</div>
							<div className="tm-chip-row">
								{[0, 100000, 200000, 500000, 1000000, 2000000].map((val) => (
									<button key={val} type="button" onClick={() => setCastFee(val)}>
										{val === 0 ? 'Freecast (0đ)' : `${new Intl.NumberFormat('vi-VN').format(val)}đ`}
									</button>
								))}
							</div>
						</>
					)}

					{stepKey === 'mode' && (
						<>
							<label className="tm-section-label">Thông tin sản phẩm & Hoa hồng đề xuất</label>
							<input
								value={campaign}
								onChange={(e) => setCampaign(e.target.value)}
								placeholder="Đợt chiến dịch (VD: Đợt T11)"
							/>
							<input
								value={productName}
								onChange={(e) => setProductName(e.target.value)}
								placeholder="Sản phẩm hợp tác"
							/>
							<div className="tm-fee-wrap">
								<input
									type="number"
									value={commissionRate}
									onChange={(e) => setCommissionRate(e.target.value)}
									placeholder="Mức hoa hồng Affiliate (%)"
								/>
								<span>%</span>
							</div>

							<label className="tm-section-label">Hoặc nếu KOC từ chối hợp tác (Chọn mã R01–R09)</label>
							<select value={declineCode} onChange={(e) => setDeclineCode(e.target.value)}>
								<option value="">-- KOC đồng ý hợp tác --</option>
								{DECLINE_REASONS.map((r) => (
									<option key={r.code} value={r.code}>
										{r.label}
									</option>
								))}
							</select>
							{declineCode && (
								<input
									value={declineNote}
									onChange={(e) => setDeclineNote(e.target.value)}
									placeholder="Ghi chú chi tiết lý do từ chối..."
								/>
							)}
						</>
					)}

					{stepKey === 'sample' && (
						<>
							<label className="tm-section-label">Đơn vị vận chuyển & Mã vận đơn</label>
							<div className="tm-grid-2">
								<select value={shippingCarrier} onChange={(e) => setShippingCarrier(e.target.value)}>
									<option value="J&T Express">J&T Express (Tích hợp API)</option>
									<option value="Tự vận chuyển">Tự vận chuyển</option>
								</select>
								<input
									type="date"
									value={shippedAt}
									onChange={(e) => setShippedAt(e.target.value)}
								/>
							</div>

							<div className="tm-lookup-row">
								<input
									value={trackingCode}
									onChange={(e) => setTrackingCode(e.target.value)}
									placeholder="Mã vận đơn J&T Express (VD: 841000012345)"
								/>
								{shippingCarrier === 'J&T Express' && (
									<button type="button" onClick={() => onTrackShipment?.(koc, trackingCode)}>
										Tra cứu J&T thật
									</button>
								)}
							</div>

							{koc.trackingStatusText && (
								<small className="tm-status-pill">
									Trạng thái vận đơn: <b>{koc.trackingStatusText}</b>
								</small>
							)}

							<label className="tm-section-label">Địa chỉ nhận mẫu của KOC (D1)</label>
							<div className="tm-grid-2">
								<input
									value={recipient}
									onChange={(e) => setRecipient(e.target.value)}
									placeholder="Tên người nhận mẫu"
									required
								/>
								<input
									value={phone}
									onChange={(e) => setPhone(e.target.value)}
									placeholder="Số điện thoại nhận hàng"
									required
								/>
							</div>
							<textarea
								rows={2}
								value={shippingAddress}
								onChange={(e) => setShippingAddress(e.target.value)}
								placeholder="Địa chỉ nhận mẫu chi tiết (VD: Ký túc xá ngoại ngữ VNU)"
								required
							/>

							{koc.stage === 'SHIPPING' && (
								<button
									type="button"
									className="tm-action-secondary"
									onClick={() =>
										onSave(
											{ action: 'mark_delivered', collabStep: 3 },
											`Đã xác nhận ${koc.name} nhận hàng thành công (DELIVERED).`,
										)
									}
								>
									✓ Xác nhận KOC đã nhận hàng (DELIVERED)
								</button>
							)}
						</>
					)}

					{stepKey === 'video' && (
						<>
							<label className="tm-section-label">Trạng thái phản hồi sau khi nhận mẫu</label>
							<div className="tm-chip-row">
								<button
									type="button"
									className={videoOutcome === 'POSTED' ? 'active-chip' : ''}
									onClick={() => setVideoOutcome('POSTED')}
								>
									Đã đăng video / Sẽ đăng
								</button>
								<button
									type="button"
									className={videoOutcome === 'NOT_FIT' ? 'active-chip' : ''}
									onClick={() => setVideoOutcome('NOT_FIT')}
								>
									Không hợp SP · Hoàn hàng (P01–P03)
								</button>
								<button
									type="button"
									className={videoOutcome === 'STALLED' ? 'active-chip danger' : ''}
									onClick={() => setVideoOutcome('STALLED')}
								>
									Quá 3 lần im lặng (Cờ đỏ STALLED)
								</button>
							</div>

							{videoOutcome === 'POSTED' && (
								<>
									<label className="tm-section-label">Link video TikTok của KOC (E2)</label>
									<div className="tm-lookup-row">
										<input
											value={videoLink}
											onChange={(e) => setVideoLink(e.target.value)}
											placeholder="https://www.tiktok.com/@.../video/..."
											required
										/>
										<button
											type="button"
											onClick={() => onSyncChannel?.(koc, { autoDetectCollabVideo: true })}
										>
											Quét kênh tự động
										</button>
									</div>

									<label className="tm-section-label">Chuỗi nhắc lịch lên video tự động (E1 · E3)</label>
									<div className="tm-chip-row">
										{[
											{ step: 1, label: 'Nhắc lần 1 (Sau 5 ngày)' },
											{ step: 2, label: 'Nhắc lần 2 (Sau 8 ngày)' },
											{ step: 3, label: 'Nhắc lần 3 (Sau 12 ngày)' },
										].map((r) => (
											<button
												key={r.step}
												type="button"
												onClick={() =>
													onSave(
														{ action: 'remind_video', videoReminderStep: r.step, collabStep: 3 },
														`Đã gửi tin nhắc lịch lên video lần ${r.step} cho ${koc.name}.`,
													)
												}
											>
												{r.label} {koc.videoReminderStep >= r.step ? '✓' : ''}
											</button>
										))}
									</div>
								</>
							)}

							{videoOutcome === 'NOT_FIT' && (
								<>
									<label className="tm-section-label">Mã lý do không lên video sau nhận hàng (F1)</label>
									<select value={postReceiveCode} onChange={(e) => setPostReceiveCode(e.target.value)}>
										{POST_RECEIVE_REASONS.map((r) => (
											<option key={r.code} value={r.code}>
												{r.label}
											</option>
										))}
									</select>
									<input
										value={postReceiveNote}
										onChange={(e) => setPostReceiveNote(e.target.value)}
										placeholder="Ghi chú lý do hoàn hàng..."
									/>

									<label className="tm-section-label">Theo dõi vận đơn hoàn hàng (F2 · RETURNING)</label>
									<div className="tm-grid-2">
										<input
											value={returnTrackingCode}
											onChange={(e) => setReturnTrackingCode(e.target.value)}
											placeholder="Mã vận đơn trả hàng"
										/>
										<input
											value={returnCondition}
											onChange={(e) => setReturnCondition(e.target.value)}
											placeholder="Tình trạng hàng trả"
										/>
									</div>

									{koc.stage === 'RETURNING' && (
										<button
											type="button"
											className="tm-action-secondary"
											onClick={() =>
												onSave(
													{ action: 'complete_returned', returnCondition, collabStep: 3 },
													`Đã xác nhận kho nhận lại hàng hoàn từ ${koc.name} (RETURNED).`,
												)
											}
										>
											✓ Xác nhận đã nhận hàng hoàn (RETURNED)
										</button>
									)}
								</>
							)}

							{videoOutcome === 'STALLED' && (
								<p className="tm-hint err">
									KOC đã nhận mẫu nhưng im lặng quá 3 lần nhắc (5 ngày / 8 ngày / 12 ngày). Bấm Xác nhận để gắn cờ đỏ STALLED lên hồ sơ.
								</p>
							)}
						</>
					)}

					{stepKey === 'result' && (
						<>
							<label className="tm-section-label">Chỉ số hiệu quả Video TikTok (E5 · E6)</label>
							<div className="tm-grid-2">
								<div>
									<small className="tm-field-caption">Lượt xem (Views)</small>
									<input
										type="number"
										value={videoViews}
										onChange={(e) => setVideoViews(e.target.value)}
										placeholder="Số lượt xem"
									/>
								</div>
								<div>
									<small className="tm-field-caption">Số đơn hàng phát sinh</small>
									<input
										type="number"
										value={videoOrders}
										onChange={(e) => setVideoOrders(e.target.value)}
										placeholder="Số đơn hàng"
									/>
								</div>
							</div>

							<label className="tm-section-label">Doanh thu (GMV) ghi nhận từ video của KOC</label>
							<div className="tm-fee-wrap">
								<input
									type="number"
									value={gmvGenerated}
									onChange={(e) => setGmvGenerated(e.target.value)}
									placeholder="Nhập GMV đạt được"
								/>
								<span>đ</span>
							</div>
						</>
					)}
				</div>

				<div className="tm-footer">
					<button type="button" className="tm-btn-cancel" onClick={onClose}>
						Huỷ
					</button>
					<button type="submit" className="tm-btn-submit">
						Xác nhận
					</button>
				</div>
			</form>
		</div>
	);
}
