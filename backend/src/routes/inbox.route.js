import { Router } from 'express';
import { z } from 'zod';
import { listConversations, listMessages, sendMessage } from '../controllers/inbox.controller.js';
import catchAsync from '../utils/catchAsync.js';
import { validate } from '../middlewares/validate.middleware.js';

const router = Router();
const messageSchema = z.object({ body: z.object({ text: z.string().min(1).max(4000) }), query: z.any(), params: z.any() });
router.get('/conversations', catchAsync(listConversations));
router.get('/conversations/:conversationId/messages', catchAsync(listMessages));
router.post('/conversations/:conversationId/messages', validate(messageSchema), catchAsync(sendMessage));
export default router;