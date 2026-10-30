import { Router } from 'express';
import { z } from 'zod';
import {
	clearConversationMessages,
	deleteSingleMessage,
	listConversations,
	listMessages,
	parseChatbotEndpoint,
	receiveInboundMessage,
	sendMessage,
} from '../controllers/inbox.controller.js';
import catchAsync from '../utils/catchAsync.js';
import { validate } from '../middlewares/validate.middleware.js';

const router = Router();
const messageSchema = z.object({
	body: z.object({
		text: z.string().min(1).max(4000),
		autoReply: z.boolean().optional(),
	}),
	query: z.any(),
	params: z.any(),
});
router.get('/conversations', catchAsync(listConversations));
router.get('/conversations/:conversationId/messages', catchAsync(listMessages));
router.post('/conversations/:conversationId/messages', validate(messageSchema), catchAsync(sendMessage));
router.post('/conversations/:conversationId/inbound', validate(messageSchema), catchAsync(receiveInboundMessage));
router.delete('/conversations/:conversationId/messages', catchAsync(clearConversationMessages));
router.delete('/conversations/:conversationId/messages/:messageId', catchAsync(deleteSingleMessage));
router.post('/chatbot/parse', catchAsync(parseChatbotEndpoint));
export default router;
