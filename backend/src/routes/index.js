import { Router } from 'express';
import authRoutes from './auth.route.js';
import inboxRoutes from './inbox.route.js';
import kocRoutes from './koc.route.js';
import { requireAuth } from '../middlewares/auth.middleware.js';

const router = Router();
router.use('/auth', authRoutes);
router.use('/kocs', requireAuth, kocRoutes);
router.use('/inbox', requireAuth, inboxRoutes);
export default router;
