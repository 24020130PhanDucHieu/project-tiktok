import { Router } from 'express';
import authRoutes from './auth.route.js';
import inboxRoutes from './inbox.route.js';
import kocRoutes from './koc.route.js';
import zaloRoutes from './zalo.route.js';
import { requireAuth } from '../middlewares/auth.middleware.js';
import { connectPersonalZalo, disconnectPersonalZalo, getPersonalZaloStatus, startPersonalZaloQr } from '../controllers/zalo.personal.controller.js';
import catchAsync from '../utils/catchAsync.js';

const router = Router();
router.use('/auth', authRoutes);
router.use('/webhooks/zalo', zaloRoutes);
router.get('/zalo/status', requireAuth, getPersonalZaloStatus);
router.post('/zalo/qr/start', requireAuth, startPersonalZaloQr);
router.post('/zalo/connect', requireAuth, catchAsync(connectPersonalZalo));
router.post('/zalo/disconnect', requireAuth, disconnectPersonalZalo);
router.use('/kocs', requireAuth, kocRoutes);
router.use('/inbox', requireAuth, inboxRoutes);
export default router;
