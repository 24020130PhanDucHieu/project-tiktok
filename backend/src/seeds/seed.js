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
	{ tiktokId: 'minhaireview', displayName: 'Minh Ái 🎀', followers: 12700, gmv: 1000000, category: 'Làm đẹp', phone: '0327 637 213' },
	{ tiktokId: 'giadinhtraicay1202', displayName: 'Gia đình trái cây 🍋🍊', followers: 3100, category: 'Đời sống', phone: '0866 073 103' },
	{ tiktokId: 'changditap', displayName: 'Cô bé lười đi tạp 🧚', followers: 126 },
]);

const [conversation] = await Conversation.create([{ channel: 'internal', contactName: 'Gia đình trái cây 🍋🍊', contactHandle: '@giadinhtraicay1202', lastMessage: 'Mình đã nhận được thông tin rồi ạ', unreadCount: 2 }]);
await Message.insertMany([
	{ conversationId: conversation._id, direction: 'inbound', text: 'Chào bạn, mình đã xem brief rồi ạ' },
	{ conversationId: conversation._id, direction: 'outbound', text: 'Chào bạn, bên mình muốn trao đổi thêm về chiến dịch mới nhé.' },
	{ conversationId: conversation._id, direction: 'inbound', text: 'Mình đã nhận được thông tin rồi ạ' },
]);
console.log('Seed completed');
await mongoose.disconnect();
