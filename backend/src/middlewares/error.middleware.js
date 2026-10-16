import { ZodError } from 'zod';

export function notFoundHandler(request, response) {
	response.status(404).json({ success: false, message: `Route not found: ${request.method} ${request.originalUrl}` });
}

export function errorHandler(error, request, response, next) {
	if (response.headersSent) return next(error);
	const statusCode = error instanceof ZodError ? 400 : error.statusCode || (error.name === 'ValidationError' ? 400 : 500);
	const message = error instanceof ZodError ? 'Dữ liệu không hợp lệ' : error.message || 'Internal server error';
	response.status(statusCode).json({ success: false, message, ...(error instanceof ZodError ? { details: error.flatten() } : {}) });
}
