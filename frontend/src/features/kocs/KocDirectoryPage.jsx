import { useMemo, useState } from 'react';
import { formatCurrency, formatNumber } from '../../lib/formatters';

export default function KocDirectoryPage({
	creators,
	folders,
	onCreateFolder,
	onOpenAddKoc,
	onSelectChat,
	onDeleteKoc,
}) {
	const [selectedFolder, setSelectedFolder] = useState('Tất cả KOC');
	const [followerFilter, setFollowerFilter] = useState('ALL');
	const [gmvFilter, setGmvFilter] = useState('ALL');
	const [contactFilter, setContactFilter] = useState('ALL');
	const [query, setQuery] = useState('');
	const [isCreateFolderOpen, setCreateFolderOpen] = useState(false);
	const [newFolderName, setNewFolderName] = useState('');

	const filteredCreators = useMemo(() => {
		return creators.filter((creator) => {
			if (selectedFolder === 'Chưa phân loại' && creator.category !== 'Chưa phân loại') return false;
			if (
				selectedFolder !== 'Tất cả KOC' &&
				selectedFolder !== 'Chưa phân loại' &&
				creator.category !== selectedFolder
			) {
				return false;
			}

			if (followerFilter === 'UNDER_10K' && creator.followers >= 10000) return false;
			if (followerFilter === '10K_50K' && (creator.followers < 10000 || creator.followers > 50000)) return false;
			if (followerFilter === 'OVER_50K' && creator.followers <= 50000) return false;

			if (gmvFilter === 'HAS_GMV' && creator.gmv <= 0) return false;
			if (gmvFilter === 'OVER_10M' && creator.gmv < 10000000) return false;

			if (contactFilter === 'HAS_PHONE' && (!creator.phone || creator.phone === '—')) return false;
			if (contactFilter === 'HAS_EMAIL' && !creator.email) return false;

			if (query.trim()) {
				const q = query.toLowerCase();
				const haystack = `${creator.name} ${creator.handle} ${creator.phone || ''} ${creator.email || ''}`.toLowerCase();
				if (!haystack.includes(q)) return false;
			}
			return true;
		});
	}, [creators, selectedFolder, followerFilter, gmvFilter, contactFilter, query]);

	const uncategorizedCount = creators.filter((c) => c.category === 'Chưa phân loại').length;

	function handleCreateFolderSubmit(e) {
		e.preventDefault();
		const name = newFolderName.trim();
		if (!name) return;
		onCreateFolder(name);
		setSelectedFolder(name);
		setNewFolderName('');
		setCreateFolderOpen(false);
	}

	return (
		<div className="koc-directory-layout">
			{/* Cột Danh bạ & Thư mục bên trái */}
			<aside className="koc-folder-panel">
				<div className="folder-section">
					<div className="folder-heading">DANH BẠ</div>
					<button
						className={selectedFolder === 'Tất cả KOC' ? 'folder-item active' : 'folder-item'}
						onClick={() => setSelectedFolder('Tất cả KOC')}
					>
						<span>Tất cả KOC</span>
						<small>{creators.length}</small>
					</button>
					<button
						className={selectedFolder === 'Chưa phân loại' ? 'folder-item active' : 'folder-item'}
						onClick={() => setSelectedFolder('Chưa phân loại')}
					>
						<span>Chưa phân loại</span>
						<small>{uncategorizedCount}</small>
					</button>
				</div>

				<div className="folder-section">
					<div className="folder-heading">THƯ MỤC</div>
					{folders.length === 0 ? (
						<p className="folder-empty">Chưa có thư mục nào.</p>
					) : (
						folders.map((folder) => {
							const count = creators.filter((c) => c.category === folder).length;
							return (
								<button
									key={folder}
									className={selectedFolder === folder ? 'folder-item active' : 'folder-item'}
									onClick={() => setSelectedFolder(folder)}
								>
									<span>{folder}</span>
									<small>{count}</small>
								</button>
							);
						})
					)}
				</div>

				<div className="folder-footer">
					{isCreateFolderOpen ? (
						<form className="inline-folder-form" onSubmit={handleCreateFolderSubmit}>
							<input
								value={newFolderName}
								onChange={(e) => setNewFolderName(e.target.value)}
								placeholder="Tên thư mục..."
								autoFocus
							/>
							<button type="submit">Lưu</button>
						</form>
					) : (
						<button className="create-folder-btn" onClick={() => setCreateFolderOpen(true)}>
							+ Tạo thư mục
						</button>
					)}
				</div>
			</aside>

			{/* Bảng Tất cả KOC bên phải */}
			<section className="koc-table-section">
				<header className="dir-header">
					<div className="dir-title">
						<h1>{selectedFolder}</h1>
						<span>{filteredCreators.length} KOC</span>
					</div>
					<div className="dir-actions">
						<button className="btn-add-koc" onClick={onOpenAddKoc}>
							+ Thêm KOC
						</button>
					</div>
				</header>

				<div className="dir-filters">
					<select value={followerFilter} onChange={(e) => setFollowerFilter(e.target.value)}>
						<option value="ALL">Follower</option>
						<option value="UNDER_10K">Dưới 10.000</option>
						<option value="10K_50K">10.000 – 50.000</option>
						<option value="OVER_50K">Trên 50.000</option>
					</select>

					<select value={gmvFilter} onChange={(e) => setGmvFilter(e.target.value)}>
						<option value="ALL">GMV</option>
						<option value="HAS_GMV">Đã có GMV</option>
						<option value="OVER_10M">Trên 10 triệu</option>
					</select>

					<select value={contactFilter} onChange={(e) => setContactFilter(e.target.value)}>
						<option value="ALL">Liên hệ</option>
						<option value="HAS_PHONE">Có số điện thoại</option>
						<option value="HAS_EMAIL">Có email</option>
					</select>

					<div className="dir-search">
						<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
							<circle cx="11" cy="11" r="7" />
							<path d="m20 20-3.5-3.5" strokeLinecap="round" />
						</svg>
						<input
							value={query}
							onChange={(e) => setQuery(e.target.value)}
							placeholder="Tìm tên, kênh, email, SĐT..."
						/>
					</div>
				</div>

				<div className="dir-table-container">
					<table>
						<thead>
							<tr>
								<th className="w-check">
									<input type="checkbox" />
								</th>
								<th className="w-index">#</th>
								<th>NHÀ SÁNG TẠO</th>
								<th>LIÊN HỆ</th>
								<th>THƯ MỤC</th>
								<th>FOLLOWER</th>
								<th>GMV</th>
								<th>PHÍ CAST</th>
								<th></th>
							</tr>
						</thead>
						<tbody>
							{filteredCreators.map((creator, idx) => (
								<tr key={creator.id}>
									<td>
										<input type="checkbox" />
									</td>
									<td className="td-index">{idx + 1}</td>
									<td>
										<div
											className="creator-cell clickable"
											onClick={() => onSelectChat(creator)}
											title="Bấm để mở Hộp thư trao đổi với KOC"
										>
											<div className="avatar-circle">{creator.avatar}</div>
											<div>
												<strong>{creator.name}</strong>
												<small>{creator.handle}</small>
											</div>
										</div>
									</td>
									<td>
										<div>{creator.contact}</div>
										{creator.email && <small className="text-muted">{creator.email}</small>}
									</td>
									<td>
										<span className="folder-pill">{creator.category}</span>
									</td>
									<td>{formatNumber(creator.followers)}</td>
									<td>{creator.gmv ? formatNumber(creator.gmv) : '—'}</td>
									<td>{formatCurrency(creator.castFee)}</td>
									<td className="row-btns">
										<button className="btn-text-link" onClick={() => onSelectChat(creator)}>
											Nhắn tin
										</button>
										<button
											className="btn-row-del"
											onClick={() => onDeleteKoc(creator)}
											title="Xóa KOC"
										>
											×
										</button>
									</td>
								</tr>
							))}
						</tbody>
					</table>

					{filteredCreators.length === 0 && (
						<div className="dir-empty-state">
							<div className="dir-empty-copy">
								<h3>Chưa có KOC nào trong danh bạ</h3>
								<p>Thêm KOC để bắt đầu xây dựng danh bạ của bạn.</p>
								<button className="btn-add-koc" onClick={onOpenAddKoc}>
									+ Thêm KOC
								</button>
							</div>
						</div>
					)}
				</div>

				<footer className="dir-footer">
					<span>
						Hiển thị{' '}
						<b>
							{filteredCreators.length ? 1 : 0}-{filteredCreators.length}
						</b>{' '}
						/ {filteredCreators.length}
					</span>
					<select defaultValue="20">
						<option value="20">20 / trang</option>
						<option value="50">50 / trang</option>
					</select>
					<div className="dir-pagination">
						<button disabled>‹</button>
						<button className="active">1</button>
						<button disabled>›</button>
					</div>
				</footer>
			</section>
		</div>
	);
}

