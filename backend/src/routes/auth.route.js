import { Router } from 'express';
import { z } from 'zod';
import { login, me, register } from '../controllers/auth.controller.js';
import { requireAuth } from '../middlewares/auth.middleware.js';
import { validate } from '../middlewares/validate.middleware.js';
import catchAsync from '../utils/catchAsync.js';

const router = Router();
const registerSchema = z.object({ body: z.object({ name: z.string().min(2).max(120), email: z.string().email(), password: z.string().min(8).max(128) }), query: z.any(), params: z.any() });
const loginSchema = z.object({ body: z.object({ email: z.string().email(), password: z.string().min(8).max(128) }), query: z.any(), params: z.any() });
router.post('/register', validate(registerSchema), catchAsync(register));
router.post('/login', validate(loginSchema), catchAsync(login));
router.get('/me', requireAuth, catchAsync(me));
export default router;
