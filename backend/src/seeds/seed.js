import bcrypt from 'bcryptjs';
import mongoose from 'mongoose';
import { connectDatabase } from '../config/db.js';
import Conversation from '../models/conversation.model.js';
import Koc from '../models/koc.model.js';
import Message from '../models/message.model.js';
import User from '../models/user.model.js';

await connectDatabase();
await Promise.all([User.deleteMany({}), Koc.deleteMany({}), Conversation.deleteMany({}), Message.deleteMany({})]);
await User.create({ name: 'Workspace Admin', email: 'admin@tiktoday.local', passwordHash: await bcrypt.hash('Admin12345!', 12), role: 'admin' });

await Koc.insertMany([
	{
		tiktokId: 'minhaireview',
		displayName: 'Minh Ái 🎀',
		followers: 12700,
		gmv: 1000000,
		category: 'Làm đẹp',
		contentPillar: 'Review skincare & unbox mỹ phẩm',
		targetAudience: 'Nữ 18-28 tuổi',
		source: 'TikToday',
		phone: '0327637213',
		campaign: 'Đợt T11 - Serum B5',
		productName: 'Serum phục hồi B5',
		commissionRate: 15,
		castType: 'freecast',
		castFee: 0,
		stage: 'QUALIFIED',
		daHopTac: false,
	},
	{
		tiktokId: 'giadinhtraicay1202',
		displayName: 'Gia đình trái cây 🍋🍊',
		followers: 3100,
		gmv: 5000000,
		category: 'Đời sống',
		contentPillar: 'Vlog nấu ăn gia đình',
		targetAudience: 'Mẹ bỉm & nội trợ 24-35',
		source: 'TikTok Liên kết',
		phone: '0866073103',
		campaign: 'Đợt T11 - Máy ép chậm',
		productName: 'Máy ép chậm mini',
		commissionRate: 10,
		castType: 'cast',
		castFee: 500000,
		stage: 'AWAITING_APPROVAL',
		daHopTac: false,
	},
	{
		tiktokId: 'changditap',
		displayName: 'Cô bé lười đi tập 🧚',
		followers: 126,
		gmv: 0,
		category: 'Chưa phân loại',
		source: 'TikTok',
		stage: 'SOURCED',
		daHopTac: false,
	},
]);

const [conversation] = await Conversation.create([{ channel: 'internal', contactName: 'Gia đình trái cây 🍋🍊', contactHandle: '@giadinhtraicay1202', lastMessage: 'Mình đã nhận được thông tin rồi ạ', unreadCount: 2 }]);
await Message.insertMany([
	{ conversationId: conversation._id, direction: 'inbound', text: 'Chào bạn, mình đã xem brief rồi ạ' },
	{ conversationId: conversation._id, direction: 'outbound', text: 'Chào bạn, bên mình muốn trao đổi thêm về chiến dịch mới nhé.' },
	{ conversationId: conversation._id, direction: 'inbound', text: 'Mình đã nhận được thông tin rồi ạ' },
]);
console.log('Seed completed');
await mongoose.disconnect();
