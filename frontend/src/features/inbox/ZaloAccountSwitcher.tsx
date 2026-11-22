import { useState } from 'react';
import Chevron from '../../components/Chevron';
import type { ZaloAccount } from '../../types/zalo';
import { accountStatusText, accountTone, switcherSummary } from './zaloAccounts';

interface Props {
	accounts: ZaloAccount[];
	/** the account the conversation list is narrowed to; "" shows every account */
	filter: string;
	onFilter: (accountId: string) => void;
	/** log an account in with a QR code */
	onConnect: (account: ZaloAccount) => void;
	onLogout: (account: ZaloAccount) => void;
	isAdmin: boolean;
	/** adding or removing an account is for admins only (the server refuses anybody else too) */
	onAdd: (label: string) => Promise<void>;
	onRemove: (account: ZaloAccount) => void;
	/** conversations that still talk through each account (by id): an account in use cannot be removed (the server refuses too) */
	conversationsByAccount?: Record<string, number>;
}

/** The Zalo chip at the top of the inbox: which accounts there are, whether each is logged in, and what can be done with it. */
export default function ZaloAccountSwitcher({ accounts, filter, onFilter, onConnect, onLogout, isAdmin, onAdd, onRemove, conversationsByAccount = {} }: Props) {
	const [open, setOpen] = useState(false);
	const [adding, setAdding] = useState(false);
	const [label, setLabel] = useState('');
	const [busy, setBusy] = useState(false);
	const sum = switcherSummary(accounts);
	const filtered = accounts.find((a) => a._id === filter);

	/** Every action closes the menu first: it would otherwise stay open behind the window the action brings up. */
	function act(run: () => void) {
		setOpen(false);
		run();
	}

	async function submitAdd() {
		const name = label.trim();
		if (!name || busy) return;
		setBusy(true);
		try {
			await onAdd(name);
			setLabel('');
			setAdding(false);
			setOpen(false);
		} finally {
			setBusy(false);
		}
	}

	return (
		<div className="zalo-switcher-wrap">
			<div
				className={`zalo-account-switcher clickable${sum.tone === 'ok' ? '' : ' off'}`}
				role="button"
				tabIndex={0}
				aria-expanded={open}
				onClick={() => setOpen((v) => !v)}
				onKeyDown={(e) => (e.key === 'Enter' || e.key === ' ') && setOpen((v) => !v)}
				title="Quản lý các tài khoản Zalo"
			>
				<div className="zalo-badge-icon">Zalo</div>
				<div className="zalo-acc-meta">
					<strong>{filtered ? filtered.label : sum.title}</strong>
					<small>{filtered ? `Đang xem riêng · ${accountStatusText(filtered)}` : sum.sub}</small>
				</div>
				<span className="caret">
					<Chevron size={16} />
				</span>
			</div>
			{open && (
				<>
					<div className="zalo-menu-backdrop" onClick={() => setOpen(false)} />
					<div className="zalo-menu zalo-accounts-menu" role="menu">
						{accounts.length > 1 && (
							<button
								type="button"
								className={`msg-menu-link${filter === '' ? ' zalo-acc-current' : ''}`}
								role="menuitem"
								onClick={() => {
									onFilter('');
									setOpen(false);
								}}
							>
								Tất cả tài khoản
							</button>
						)}
						{accounts.map((a) => (
							<div key={a._id} className={`zalo-acc-row${filter === a._id ? ' zalo-acc-current' : ''}`}>
								<button
									type="button"
									className="zalo-acc-pick"
									role="menuitem"
									title={accounts.length > 1 ? 'Chỉ xem hội thoại của tài khoản này' : undefined}
									onClick={() => {
										onFilter(accounts.length > 1 && filter !== a._id ? a._id : '');
										setOpen(false);
									}}
								>
									<i className={`zalo-dot ${accountTone(a)}`} aria-hidden="true" />
									<span className="zalo-acc-text">
										<strong>
											<span className="zalo-acc-name">{a.label}</span>
											{a.isDefault && accounts.length > 1 && <em>mặc định</em>}
										</strong>
										<small>{accountStatusText(a)}</small>
									</span>
								</button>
								<span className="zalo-acc-actions">
									{a.connected ? (
										<button type="button" className="zalo-acc-btn" onClick={() => act(() => onLogout(a))}>
											Đăng xuất
										</button>
									) : (
										<button type="button" className="zalo-acc-btn primary" onClick={() => act(() => onConnect(a))}>
											{a.needsQr ? 'Quét QR' : 'Kết nối lại'}
										</button>
									)}
									{isAdmin && (
										<button
											type="button"
											className="zalo-acc-btn danger"
											disabled={(conversationsByAccount[a._id] ?? 0) > 0}
											title={(conversationsByAccount[a._id] ?? 0) > 0 ? `Còn ${conversationsByAccount[a._id]} cuộc trò chuyện đang dùng tài khoản này nên chưa xóa được` : 'Xóa tài khoản này khỏi hệ thống'}
											onClick={() => act(() => onRemove(a))}
										>
											Xóa
										</button>
									)}
								</span>
							</div>
						))}
						{isAdmin &&
							(adding ? (
								<form
									className="zalo-acc-add"
									onSubmit={(e) => {
										e.preventDefault();
										void submitAdd();
									}}
								>
									<input
										autoFocus
										value={label}
										maxLength={60}
										onChange={(e) => setLabel(e.target.value)}
										placeholder="Tên gọi, ví dụ: Zalo phụ"
										aria-label="Tên tài khoản Zalo mới"
									/>
									<button type="submit" className="zalo-acc-btn primary" disabled={!label.trim() || busy}>
										Thêm và quét QR
									</button>
								</form>
							) : (
								<button type="button" className="msg-menu-link zalo-acc-addlink" role="menuitem" onClick={() => setAdding(true)}>
									+ Thêm tài khoản Zalo
								</button>
							))}
					</div>
				</>
			)}
		</div>
	);
}
