import { COLLAB_STEPS, initialCreators } from '../../lib/constants';
import { formatCurrency, formatNumber } from '../../lib/formatters';

export default function InspectorPanel({
	rightTab,
	setRightTab,
	activeKoc,
	currentStepIndex,
	expandedStepKey,
	setExpandedStepKey,
	setActiveStepModal,
	tiktokShopLinked,
	shopName,
	setShowLinkShopModal,
	syncingChannel,
	setSyncingChannel,
	onSyncChannel,
	channelFilterCart,
	setChannelFilterCart,
	channelFilterType,
	setChannelFilterType,
	channelSortOrder,
	setChannelSortOrder,
}) {
	return (
		<aside className="msg-col-inspector">
			<div className="inspector-tabs">
				<button
					className={rightTab === 'Hợp tác' ? 'active' : ''}
					onClick={() => setRightTab('Hợp tác')}
				>
					Hợp tác
				</button>
				<button
					className={rightTab === 'Kênh' ? 'active' : ''}
					onClick={() => setRightTab('Kênh')}
				>
					Kênh
				</button>
			</div>

			{rightTab === 'Hợp tác' ? (
				<div className="inspector-body">
					{activeKoc?.stalledFlag && (
						<div className="alert-banner red">
							<strong>Cờ đỏ · KOC quá 3 lần nhắc lịch video (STALLED)</strong>
							<small>KOC im lặng sau 3 mốc nhắc (+5N / +8N / +12N). Đã dừng nhắc tự động.</small>
						</div>
					)}

					{activeKoc?.tungKhongRep && (
						<div className="alert-banner amber">
							<strong>Nhãn lưu ý · Từng không phản hồi ở đợt trước (#1)</strong>
							<small>KOC này từng hết chuỗi nhắc mà không trả lời ở chiến dịch trước.</small>
						</div>
					)}

					{activeKoc?.suggestedDeclineCode && activeKoc?.stage !== 'DECLINED' && (
						<div className="alert-banner amber">
							<strong>Chatbot đề xuất mã từ chối: {activeKoc.suggestedDeclineCode}</strong>
							<small>Bấm vào Bước 2 để Booker xác nhận mã lý do từ chối hoặc duyệt lại phí cast.</small>
						</div>
					)}

					{activeKoc?.dnc && (
						<div className="alert-banner red">
							<strong>Cờ DNC (Do Not Contact) đang bật</strong>
							<small>Chatbot và hệ thống tự động sẽ không gửi bất kỳ tin nhắn nào cho KOC này.</small>
						</div>
					)}

					{activeKoc?.shippingDelayedAlert && (
						<div className="alert-banner amber">
							<strong>Cảnh báo J&T Express · Quá 5 ngày chưa giao</strong>
							<small>Mã vận đơn #{activeKoc.trackingCode} đã quá 5 ngày chưa ký nhận.</small>
						</div>
					)}

					{['NOT_FIT', 'RETURNING', 'RETURNED'].includes(activeKoc?.stage) && (
						<div className="alert-banner amber">
							<strong>
								{activeKoc.stage === 'RETURNED'
									? 'Đã nhận hàng hoàn (RETURNED)'
									: `Hoàn hàng sau nhận mẫu (${activeKoc.postReceiveCode || 'P01'})`}
							</strong>
							<small>
								{activeKoc.returnTrackingCode
									? `Mã vận đơn hoàn: #${activeKoc.returnTrackingCode}`
									: 'Bấm vào bước KOC đăng bài để cập nhật mã vận đơn trả hàng.'}
							</small>
						</div>
					)}

					<div className="vertical-stepper">
						{COLLAB_STEPS.map((step, index) => {
							const isCurrent = index === currentStepIndex;
							const isCompleted = index < currentStepIndex;
							const isExpandedInline =
								(step.key === 'video' || step.key === 'result') && expandedStepKey === step.key;

							return (
								<div
									key={step.key}
									className={`v-step ${isCurrent ? 'current' : ''} ${isCompleted ? 'done' : ''}`}
								>
									<div className="v-step-rail">
										<span className="v-step-dot" />
										{index < COLLAB_STEPS.length - 1 && <span className="v-step-line" />}
									</div>
									<div className="v-step-content">
										<div
											className="v-step-title"
											onClick={() => {
												if (step.key === 'video' || step.key === 'result') {
													setExpandedStepKey((prev) => (prev === step.key ? null : step.key));
												} else {
													setActiveStepModal(step.key);
												}
											}}
										>
											{step.title}
										</div>

										{isCompleted && step.key === 'cast' && (
											<small className="v-step-summary">
												{activeKoc?.castFee > 0
													? `Đã chốt: ${formatCurrency(activeKoc.castFee)}`
													: 'Đã chốt: Freecast (0đ)'}
											</small>
										)}
										{isCompleted && step.key === 'mode' && (
											<small className="v-step-summary">
												{activeKoc?.productName || 'Sản phẩm mẫu'} · HH {activeKoc?.commissionRate || 10}%
											</small>
										)}
										{isCompleted && step.key === 'sample' && (
											<small className="v-step-summary">
												Giao tới: {activeKoc?.shippingAddress || 'Đã duyệt mẫu'}
												{activeKoc?.trackingCode
													? ` (${activeKoc.shippingCarrier || 'J&T'}: #${activeKoc.trackingCode})`
													: ''}
											</small>
										)}
										{isCompleted && step.key === 'video' && (
											<small className="v-step-summary">
												{activeKoc?.videoUrl ? 'Đã đăng video TikTok' : 'Đã cập nhật bài đăng'}
												{activeKoc?.videoViews ? ` · ${formatNumber(activeKoc.videoViews)} views` : ''}
											</small>
										)}
										{isCompleted && step.key === 'result' && (
											<small className="v-step-summary">
												GMV: {formatCurrency(activeKoc?.videoGmv || activeKoc?.gmv || 0)}
												{activeKoc?.videoOrders ? ` · ${activeKoc.videoOrders} đơn` : ''}
											</small>
										)}

										{isCurrent && (
											<button
												type="button"
												className="v-step-btn"
												onClick={() => setActiveStepModal(step.key)}
											>
												{step.buttonLabel}
											</button>
										)}

										{/* KHỐI INLINE "LIÊN KẾT CỬA HÀNG TIKTOK SHOP" Ở BƯỚC 4 & BƯỚC 5 */}
										{isExpandedInline && !tiktokShopLinked && (
											<div className="tts-connect-card">
												<div className="tts-store-icon-box">
													<svg
														width="22"
														height="22"
														viewBox="0 0 24 24"
														fill="none"
														stroke="currentColor"
														strokeWidth="1.8"
														strokeLinecap="round"
														strokeLinejoin="round"
													>
														<path d="M3 9l1.5-5h15L21 9" />
														<path d="M3 9c0 1.66 1.34 3 3 3s3-1.34 3-3c0 1.66 1.34 3 3 3s3-1.34 3-3c0 1.66 1.34 3 3 3s3-1.34 3-3" />
														<path d="M5 12v8a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1v-8" />
														<path d="M9 21v-6h6v6" />
													</svg>
												</div>
												<h4>Liên kết cửa hàng TikTok Shop</h4>
												<p>
													{step.key === 'video'
														? 'Liên kết rồi, chặng này hiện đúng những bài KOC đã đăng cho sản phẩm của lần hợp tác — kèm lượt xem và doanh thu từng bài mang về.'
														: 'Liên kết rồi, chặng này hiện doanh thu, hoa hồng và lãi/lỗ thật của lần hợp tác — thay vì mở TikTok Shop ra cộng tay.'}
												</p>
												<button
													type="button"
													className="tts-connect-btn"
													onClick={() => setShowLinkShopModal(true)}
												>
													Liên kết cửa hàng
												</button>
												<button
													type="button"
													className="tts-manual-link"
													onClick={() => setActiveStepModal(step.key)}
												>
													Hoặc cập nhật thủ công →
												</button>
											</div>
										)}

										{isExpandedInline && tiktokShopLinked && step.key === 'video' && (
											<div className="tts-linked-summary">
												<div className="tts-linked-badge">
													<span>● Đã liên kết: {shopName}</span>
													<button type="button" onClick={() => setActiveStepModal('video')}>
														Cập nhật
													</button>
												</div>
												<div className="tts-linked-post">
													<strong>
														{activeKoc?.productName || 'Khăn lạnh thể thao Cooltech'}
													</strong>
													<small>
														Lượt xem: <b>{formatNumber(activeKoc?.videoViews || 4200)}</b> · Doanh thu bài:{' '}
														<b>{formatCurrency(activeKoc?.videoGmv || activeKoc?.gmv || 12500000)}</b>
													</small>
												</div>
											</div>
										)}

										{isExpandedInline && tiktokShopLinked && step.key === 'result' && (
											<div className="tts-linked-summary">
												<div className="tts-linked-badge">
													<span>● Đối soát từ {shopName}</span>
													<button type="button" onClick={() => setActiveStepModal('result')}>
														Chỉnh sửa
													</button>
												</div>
												{(() => {
													const rev = Number(activeKoc?.videoGmv || activeKoc?.gmv || 12500000);
													const comm = Math.round((rev * (Number(activeKoc?.commissionRate) || 10)) / 100);
													const fee = Number(activeKoc?.castFee) || 0;
													const profit = rev - comm - fee;
													return (
														<div className="tts-pnl-grid">
															<div>
																<span>Doanh thu (GMV)</span>
																<strong>{formatCurrency(rev)}</strong>
															</div>
															<div>
																<span>Hoa hồng ({activeKoc?.commissionRate || 10}%)</span>
																<strong>{formatCurrency(comm)}</strong>
															</div>
															<div>
																<span>Phí cast</span>
																<strong>{fee ? formatCurrency(fee) : '0đ'}</strong>
															</div>
															<div>
																<span>Lãi / Lỗ tạm tính</span>
																<strong className={profit >= 0 ? 'text-green' : 'text-red'}>
																	{profit >= 0 ? `+${formatCurrency(profit)}` : formatCurrency(profit)}
																</strong>
															</div>
														</div>
													);
												})()}
											</div>
										)}
									</div>
								</div>
							);
						})}
					</div>
				</div>
			) : (
				<div className="inspector-body channel-tab-body">
					<div className="channel-sync-note">
						<span className={syncingChannel ? 'sync-spinner spinning' : 'sync-spinner'} />
						<span>
							{syncingChannel
								? 'Đang lấy dữ liệu thật từ TikTok...'
								: 'Đang cập nhật dữ liệu kênh · dữ liệu sẽ tự làm mới khi xong.'}
						</span>
						<button
							type="button"
							className="btn-refresh-channel"
							onClick={() => {
								setSyncingChannel(true);
								Promise.resolve(onSyncChannel?.(activeKoc)).finally(() =>
									setTimeout(() => setSyncingChannel(false), 500),
								);
							}}
						>
							Làm mới
						</button>
					</div>

					<div className="channel-stat-cards">
						<div className="ch-stat-box">
							<span>Follower</span>
							<strong>{formatNumber(activeKoc?.followers || 4000)}</strong>
						</div>
						<div className="ch-stat-box">
							<span>GMV 30 ngày ⓘ</span>
							<strong>{formatNumber(activeKoc?.gmv || 125500000)}</strong>
						</div>
					</div>

					<div className="ch-video-header">
						<span>VIDEO · 60 GẦN NHẤT</span>
						<small>
							Quét{' '}
							{activeKoc?.channelSyncedAt
								? new Date(activeKoc.channelSyncedAt).toLocaleDateString('vi-VN')
								: new Date().toLocaleDateString('vi-VN')}
						</small>
					</div>

					<select
						className="ch-select-full"
						value={channelFilterCart}
						onChange={(e) => setChannelFilterCart(e.target.value)}
					>
						<option value="ALL">Tất cả</option>
						<option value="AFF">Có gắn giỏ hàng</option>
					</select>

					<div className="ch-select-row">
						<select
							value={channelFilterType}
							onChange={(e) => setChannelFilterType(e.target.value)}
						>
							<option value="ALL">Loại video</option>
							<option value="REVIEW">Review / Unbox</option>
							<option value="OUTFIT">Phối đồ / Thể thao</option>
						</select>
						<select
							value={channelSortOrder}
							onChange={(e) => setChannelSortOrder(e.target.value)}
						>
							<option value="NEWEST">Mới nhất</option>
							<option value="MOST_VIEWED">Lượt xem cao nhất</option>
						</select>
					</div>

					<div className="ch-video-list">
						{(() => {
							const rawList = activeKoc?.videos?.length
								? [...activeKoc.videos]
								: [...initialCreators[0].videos];

							const filteredList = rawList.filter((v) => {
								if (channelFilterCart === 'AFF' && !v.product) return false;
								if (
									channelFilterType === 'REVIEW' &&
									!/(review|skincare|unbox|make up)/i.test(`${v.title} ${v.product}`)
								) {
									return false;
								}
								if (
									channelFilterType === 'OUTFIT' &&
									!/(chạy bộ|thể thao|phối|áo|quần|giày)/i.test(`${v.title} ${v.product}`)
								) {
									return false;
								}
								return true;
							});

							if (channelSortOrder === 'MOST_VIEWED') {
								filteredList.sort((a, b) => {
									const parseV = (s) =>
										parseFloat(String(s || '0').replace(/k/i, '000').replace(/m/i, '000000')) || 0;
									return parseV(b.views) - parseV(a.views);
								});
							}

							return filteredList.map((v) => (
								<a
									key={v.id}
									className="ch-video-item"
									href={v.url || `https://www.tiktok.com/${activeKoc?.handle || ''}`}
									target="_blank"
									rel="noreferrer"
								>
									<div
										className="ch-video-thumb"
										style={
											v.coverUrl
												? {
														backgroundImage: `linear-gradient(rgba(15,23,42,0.12), rgba(15,23,42,0.45)), url(${v.coverUrl})`,
														backgroundSize: 'cover',
														backgroundPosition: 'center',
														color: '#ffffff',
													}
												: undefined
										}
									>
										{!v.coverUrl && <span className="play-triangle">▷</span>}
										{!v.coverUrl && <small>{v.date}</small>}
										<span className="ch-thumb-store-badge" title="Video có gắn giỏ hàng TikTok Shop">
											<svg
												width="11"
												height="11"
												viewBox="0 0 24 24"
												fill="none"
												stroke="currentColor"
												strokeWidth="2.4"
											>
												<path d="M3 9l1.5-5h15L21 9" />
												<path d="M5 12v8h14v-8" />
											</svg>
										</span>
									</div>
									<div className="ch-video-info">
										<strong>{v.title}</strong>
										<small className="ch-video-metrics">
											<span>👁 {v.views}</span>
											<span>♡ {v.likes === '—' ? 0 : v.likes}</span>
											<span>💬 {v.comments === '—' ? 0 : v.comments}</span>
											<span>↱ {v.shares === '—' ? 1 : v.shares}</span>
										</small>
										<small className="ch-video-prod">
											<span className="ch-prod-mini-icon">🛍</span>
											<span>{v.product || activeKoc?.productName || 'Sản phẩm gắn giỏ hàng'}</span>
										</small>
									</div>
								</a>
							));
						})()}
					</div>
				</div>
			)}
		</aside>
	);
}

