import app from './app.js';
import { connectDatabase } from './config/db.js';
import env from './config/env.js';

try {
	await connectDatabase();
	app.listen(env.port, () => console.log(`API listening on http://127.0.0.1:${env.port}`));
} catch (error) {
	console.error('Unable to start API', error);
	process.exitCode = 1;
}
