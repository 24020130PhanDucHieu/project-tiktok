import { type ChangeEvent, type FormEvent, useState } from 'react';
import api from '../../lib/axios';
import { formatNumber } from '../../lib/formatters';
import type { VideoItem } from '../../types/koc';
import Select from '../../components/Select';

interface FoundChannelInfo {
	tiktokId: string;
	displayName: string;
	avatarUrl?: string;
	followers: number;
	gmv: number;
	videos?: VideoItem[];
}

interface AddKocModalProps {
	folders: string[];
	onClose: () => void;
	onAdd: (payload: Record<string, any>) => void;
}

export default function AddKocModal({ folders, onClose, onAdd }: AddKocModalProps) {
	const [tiktokId, setTiktokId] = useState('');
	const [foundChannel, setFoundChannel] = useState<FoundChannelInfo | null>(null);
	const [lookupMsg, setLookupMsg] = useState('');
	const [lookupErr, setLookupErr] = useState(false);

	const [form, setForm] = useState({
		email: '',
		phone: '',
		recipient: '',
		shippingAddress: '',
		castFee: '',
		category: 'Chưa phân loại',
	});

	const cleanTyped = tiktokId.trim().replace(/^@/, '').toLowerCase();
	// a KOC can only be added after the channel was found (and verified in TikTok Shop) for exactly this ID
	const verified = !!foundChannel && foundChannel.tiktokId === cleanTyped;

	function handleLookup() {
		const cleanId = tiktokId.trim().replace(/^@/, '').toLowerCase();
		if (!cleanId) {
			setLookupErr(true);
			setLookupMsg('Vui lòng nhập TikTok ID trước khi bấm Tìm kênh.');
			return;
		}
		api.post('/kocs/lookup', { tiktokId: cleanId, phone: form.phone })
			.then(({ data }) => {
				const res = data.data;
				if (res.blocked || res.duplicate) {
					setFoundChannel(null);
					setLookupErr(true);
					setLookupMsg(res.message || 'Kênh này đã có trong danh bạ.');
				} else {
					const prof = res.profile || {};
					setFoundChannel({
						tiktokId: cleanId,
						displayName: prof.displayName || cleanId,
						avatarUrl: prof.avatarUrl || '',
						followers: prof.followers || 0,
						gmv: prof.gmv || 0,
						videos: res.videos || [],
					});
					setForm((prev) => ({
						...prev,
						recipient: prev.recipient || prof.displayName || '',
						phone: prev.phone || prof.phone || '',
						email: prev.email || prof.email || '',
					}));
					setLookupErr(false);
					setLookupMsg(
						res.message ||
							`Đã tìm thấy kênh thật @${cleanId} (${formatNumber(prof.followers || 0)} follower)`,
					);
				}
			})
			.catch((e) => {
				setFoundChannel(null);
				setLookupErr(true);
				setLookupMsg(e?.response?.data?.message || 'Không xác minh được kênh lúc này. Thử lại sau ít phút.');
			});
	}

	function handleChange(e: ChangeEvent<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>) {
		setForm({ ...form, [e.target.name]: e.target.value });
	}

	function handleSubmit(e: FormEvent<HTMLFormElement>) {
		e.preventDefault();
		const cleanId = cleanTyped;
		if (!cleanId) return;
		if (!verified || !foundChannel) {
			setLookupErr(true);
			setLookupMsg('Bấm "Tìm kênh" để xác minh nhà sáng tạo có trong TikTok Shop trước khi thêm.');
			return;
		}

		onAdd({
			tiktokId: cleanId,
			displayName: foundChannel.displayName || form.recipient || cleanId,
			avatarUrl: foundChannel.avatarUrl || '',
			followers: foundChannel.followers || 0,
			gmv: foundChannel.gmv || 0,
			videos: foundChannel.videos || [],
			email: form.email,
			phone: form.phone,
			recipient: form.recipient,
			shippingAddress: form.shippingAddress,
			castFee: Number(form.castFee) || 0,
			category: form.category,
		});
	}

	return (
		<div className="modal-backdrop" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
			<form className="tiktoday-modal" onSubmit={handleSubmit}>
				<div className="tm-header">
					<div>
						<h2>Thêm KOC vào danh bạ</h2>
						<p>Tìm theo TikTok ID rồi chọn kênh để lưu kèm follower & GMV</p>
					</div>
					<button type="button" className="tm-close" onClick={onClose}>
						×
					</button>
				</div>

				<div className="tm-body">
					<div className="tm-lookup-row">
						<input
							value={tiktokId}
							onChange={(e) => {
								setTiktokId(e.target.value);
								setFoundChannel(null);
								setLookupMsg('');
								setLookupErr(false);
							}}
							placeholder="TikTok ID *"
						/>
						<button type="button" onClick={handleLookup}>
							Tìm kênh
						</button>
					</div>
					<small className={`tm-hint ${lookupErr ? 'err' : ''}`}>
						{lookupMsg || 'Bấm "Tìm kênh": chỉ thêm được nhà sáng tạo có trong TikTok Shop'}
					</small>

					<label className="tm-section-label">Thông tin liên hệ</label>
					<input
						name="email"
						value={form.email}
						onChange={handleChange}
						placeholder="Email"
					/>
					<input
						name="phone"
						value={form.phone}
						onChange={handleChange}
						placeholder="Số điện thoại"
					/>
					<input
						name="recipient"
						value={form.recipient}
						onChange={handleChange}
						placeholder="Người nhận mẫu"
					/>
					<textarea
						rows={2}
						name="shippingAddress"
						value={form.shippingAddress}
						onChange={handleChange}
						placeholder="Địa chỉ nhận mẫu"
					/>

					<div className="tm-fee-wrap">
						<input
							name="castFee"
							type="number"
							value={form.castFee}
							onChange={handleChange}
							placeholder="Phí cast"
						/>
						<span>đ</span>
					</div>

					<div className="tm-chip-row">
						{[100000, 200000, 500000, 1000000, 2000000].map((fee) => (
							<button
								key={fee}
								type="button"
								onClick={() => setForm({ ...form, castFee: String(fee) })}
							>
								{new Intl.NumberFormat('vi-VN').format(fee)}đ
							</button>
						))}
					</div>

					<label className="tm-section-label">Thư mục</label>
					<Select name="category" value={form.category} onChange={handleChange}>
						<option value="Chưa phân loại">Không thêm vào thư mục</option>
						{folders.map((f) => (
							<option key={f} value={f}>
								{f}
							</option>
						))}
					</Select>
				</div>

				<div className="tm-footer">
					<button type="button" className="tm-btn-cancel" onClick={onClose}>
						Huỷ
					</button>
					<button type="submit" className="tm-btn-submit" disabled={!verified} title={verified ? undefined : 'Cần tìm kênh và xác minh trước'}>
						+ Thêm KOC
					</button>
				</div>
			</form>
		</div>
	);
}

