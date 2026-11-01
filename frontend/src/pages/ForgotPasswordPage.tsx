import ResetCodeForm from '../components/ResetCodeForm';
import AuthShell from './AuthShell';

export default function ForgotPasswordPage({ onBack }: { onBack: () => void }) {
	return (
		<AuthShell>
			<div className="login-card">
				<p className="eyebrow">KHÔI PHỤC TÀI KHOẢN</p>
				<h2>Quên mật khẩu?</h2>
				<p className="login-muted">Nhập email đăng nhập. Chúng tôi sẽ gửi mã xác nhận 6 số (hiệu lực 10 phút) để bạn đặt mật khẩu mới.</p>
				<ResetCodeForm onDone={onBack} />
				<button type="button" className="auth-link" onClick={onBack}>
					Quay lại đăng nhập
				</button>
			</div>
		</AuthShell>
	);
}
