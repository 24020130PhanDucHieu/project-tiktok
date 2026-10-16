import { Router } from 'express';
import catchAsync from '../utils/catchAsync.js';
import { receiveZaloWebhook, verifyZaloWebhook } from '../controllers/zalo.webhook.controller.js';

const router = Router();
router.get('/', verifyZaloWebhook);
router.post('/', catchAsync(receiveZaloWebhook));
export default router;