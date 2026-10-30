import { Router } from 'express';
import { z } from 'zod';
import {
	archiveKoc,
	bulkImportKocs,
	createKoc,
	emailKoc,
	listKocs,
	lookupTikTok,
	runOutreachAutomation,
	updateKocStage,
} from '../controllers/koc.controller.js';
import { KOC_STAGES } from '../models/koc.model.js';
import catchAsync from '../utils/catchAsync.js';
import { validate } from '../middlewares/validate.middleware.js';

const router = Router();

const createSchema = z.object({
	body: z.object({
		tiktokId: z.string().min(2).max(100),
		displayName: z.string().min(1).max(120),
		followers: z.number().int().nonnegative().optional(),
		gmv: z.number().nonnegative().optional(),
		category: z.string().max(80).optional(),
		contentPillar: z.string().max(150).optional(),
		targetAudience: z.string().max(150).optional(),
		source: z.enum(['TikToday', 'TikTok Liên kết', 'TikTok']).optional(),
		email: z.string().email().optional().or(z.literal('')),
		phone: z.string().max(30).optional(),
		recipient: z.string().max(120).optional(),
		shippingAddress: z.string().max(300).optional(),
		daHopTac: z.boolean().optional(),
		campaign: z.string().max(120).optional(),
		productName: z.string().max(150).optional(),
		commissionRate: z.number().min(0).max(100).optional(),
		castFee: z.number().nonnegative().optional(),
		stage: z.enum(KOC_STAGES).optional(),
	}),
	query: z.any(),
	params: z.any(),
});

const bulkImportSchema = z.object({
	body: z.object({
		source: z.enum(['TikToday', 'TikTok Liên kết', 'TikTok']).optional(),
		items: z
			.array(
				z.object({
					tiktokId: z.string().min(2).max(100),
					displayName: z.string().max(120).optional(),
					phone: z.string().max(30).optional(),
					source: z.enum(['TikToday', 'TikTok Liên kết', 'TikTok']).optional(),
					category: z.string().max(80).optional(),
					followers: z.number().nonnegative().optional(),
					gmv: z.number().nonnegative().optional(),
					castFee: z.number().nonnegative().optional(),
				}),
			)
			.min(1),
	}),
	query: z.any(),
	params: z.any(),
});

const stageUpdateSchema = z.object({
	body: z.object({
		action: z.enum([
			'qualify',
			'approve_cast',
			'filter_out',
			'toggle_collaborated',
			'send_invite',
			'remind_step',
			'mark_replied',
			'mark_dormant',
			'reopen_dormant',
			'agree',
			'decline',
			'set_stage',
		]),
		contentPillar: z.string().max(150).optional(),
		targetAudience: z.string().max(150).optional(),
		campaign: z.string().max(120).optional(),
		productName: z.string().max(150).optional(),
		commissionRate: z.number().min(0).max(100).optional(),
		castFee: z.number().nonnegative().optional(),
		rejectCode: z.string().max(50).optional(),
		rejectNote: z.string().max(300).optional(),
		declineCode: z.string().max(50).optional(),
		declineNote: z.string().max(300).optional(),
		inviteBrief: z.string().max(1000).optional(),
		reminderStep: z.number().int().min(1).max(3).optional(),
		recipient: z.string().max(120).optional(),
		shippingAddress: z.string().max(300).optional(),
		phone: z.string().max(30).optional(),
		daHopTac: z.boolean().optional(),
		stage: z.enum(KOC_STAGES).optional(),
	}),
	query: z.any(),
	params: z.any(),
});

const outreachAutomationSchema = z.object({
	body: z.object({
		mode: z.enum(['invite_all_qualified', 'sweep_followups']),
		simulateNextStep: z.boolean().optional(),
		inviteBrief: z.string().max(1000).optional(),
	}),
	query: z.any(),
	params: z.any(),
});

const lookupSchema = z.object({
	body: z.object({
		tiktokId: z.string().min(2).max(100),
		phone: z.string().max(30).optional(),
	}),
	query: z.any(),
	params: z.any(),
});

const emailSchema = z.object({
	body: z.object({ text: z.string().max(4000).optional() }),
	query: z.any(),
	params: z.any(),
});

router.get('/', catchAsync(listKocs));
router.post('/lookup', validate(lookupSchema), catchAsync(lookupTikTok));
router.post('/bulk', validate(bulkImportSchema), catchAsync(bulkImportKocs));
router.post('/outreach/run', validate(outreachAutomationSchema), catchAsync(runOutreachAutomation));
router.post('/', validate(createSchema), catchAsync(createKoc));
router.patch('/:id/stage', validate(stageUpdateSchema), catchAsync(updateKocStage));
router.post('/:id/email', validate(emailSchema), catchAsync(emailKoc));
router.delete('/:id', catchAsync(archiveKoc));

export default router;
