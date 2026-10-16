import { loginUser, registerUser } from '../services/auth.service.js';

export async function register(request, response) { response.status(201).json({ success: true, data: await registerUser(request.validated.body) }); }
export async function login(request, response) { response.json({ success: true, data: await loginUser(request.validated.body) }); }
export async function me(request, response) { response.json({ success: true, data: request.user }); }
