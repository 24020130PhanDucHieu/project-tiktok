import { Router } from 'express';
import { z } from 'zod';
import { createKoc, listKocs, lookupTikTok } from '../controllers/koc.controller.js';
import catchAsync from '../utils/catchAsync.js';
import { validate } from '../middlewares/validate.middleware.js';

const router = Router();
const createSchema = z.object({ body: z.object({ tiktokId: z.string().min(2).max(100), displayName: z.string().min(1).max(120), followers: z.number().int().nonnegative().optional(), gmv: z.number().nonnegative().optional(), category: z.string().max(80).optional(), email: z.string().email().optional().or(z.literal('')), phone: z.string().max(30).optional(), recipient: z.string().max(120).optional(), shippingAddress: z.string().max(300).optional(), castFee: z.number().nonnegative().optional() }), query: z.any(), params: z.any() });
const lookupSchema = z.object({ body: z.object({ tiktokId: z.string().min(2).max(100) }), query: z.any(), params: z.any() });
router.get('/', catchAsync(listKocs));
router.post('/lookup', validate(lookupSchema), catchAsync(lookupTikTok));
router.post('/', validate(createSchema), catchAsync(createKoc));
export default router;