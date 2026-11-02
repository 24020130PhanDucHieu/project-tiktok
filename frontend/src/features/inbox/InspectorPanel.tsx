import type { Dispatch, ReactNode, SetStateAction } from 'react';
import { COLLAB_STEPS, initialCreators } from '../../lib/constants';
import { formatCurrency, formatNumber } from '../../lib/formatters';
import type { CollabStepKey, Creator } from '../../types/koc';
import ChannelTab from './ChannelTab';

interface InspectorPanelProps {
	rightTab: string;
	setRightTab: (tab: string) => void;
	activeKoc: Creator | null;
	currentStepIndex: number;
	expandedStepKey: string | null;
	setExpandedStepKey: Dispatch<SetStateAction<string | null>>;
	setActiveStepModal: (stepKey: CollabStepKey | null) => void;
	tiktokShopLinked: boolean;
	shopName: string;
	setShowLinkShopModal: (show: boolean) => void;
	syncingChannel: boolean;
	setSyncingChannel: (syncing: boolean) => void;
	onSyncChannel: (creator: Creator | null, opts?: { autoDetectCollabVideo?: boolean }) => void;
	onTrackShipment: (creator: Creator | null, customTrackingCode?: string, customPhoneLast4?: string) => void;
	collabPanel: ReactNode;
}

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
	onTrackShipment,
	collabPanel,
}: InspectorPanelProps) {
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

					{activeKoc && ['NOT_FIT', 'RETURNING', 'RETURNED'].includes(activeKoc.stage) && (
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

					{collabPanel}
				</div>
			) : (
				<div className="inspector-body channel-tab-body">
					<ChannelTab koc={activeKoc} syncing={syncingChannel} setSyncing={setSyncingChannel} onSyncChannel={onSyncChannel} />
				</div>
			)}
		</aside>
	);
}

