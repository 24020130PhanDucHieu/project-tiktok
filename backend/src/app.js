import cors from 'cors';
import express from 'express';
import helmet from 'helmet';
import morgan from 'morgan';
import env from './config/env.js';
import apiRoutes from './routes/index.js';
import { errorHandler, notFoundHandler } from './middlewares/error.middleware.js';

const app = express();
app.disable('x-powered-by');
app.use(helmet());
app.use(cors({ origin: env.frontendUrl }));
app.use(express.json({ limit: '1mb' }));
app.use(morgan(env.nodeEnv === 'production' ? 'combined' : 'dev'));
app.get('/health', (request, response) => response.json({ success: true, service: 'tiktoday-api' }));
app.use('/api/v1', apiRoutes);
app.use(notFoundHandler);
app.use(errorHandler);

export default app;
