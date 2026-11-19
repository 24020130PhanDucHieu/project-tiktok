import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
	plugins: [react()],
	build: {
		rollupOptions: {
			output: {
				manualChunks(id: string) {
					// the Excel libraries are only needed when someone imports or exports a file: keep them out of the
					// vendor chunk every page loads, so they stay in the chunks that import() fetches on demand
					if (/node_modules\/(read-excel-file|write-excel-file|fflate|saxen|unzipper-esm|worker-f)\//.test(id)) return undefined;
					// the canvas of the Quy trình tab (and what it is built on) is only needed when that tab opens
					if (/node_modules\/(@xyflow|d3-[a-z-]+|zustand|classcat|use-sync-external-store)\//.test(id)) return undefined;
					if (id.includes('node_modules')) return 'vendor';
				},
			},
		},
	},
});

