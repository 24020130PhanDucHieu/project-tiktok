import { createContext, useContext } from 'react';
import type { Campaign } from '../../types/campaign';
import type { ZaloAccount } from '../../types/zalo';
import type { BulkRow } from '../kocs/bulkImport';
import type { InboxLabel, WorkflowPort } from '../../types/workflow';
import type { NodeData } from './convert';

/** The Excel file of the import block: what was read from it, kept in the editor (never stored in the browser). */
export interface SheetInfo {
	fileName: string;
	rows: BulkRow[];
}

/** What a block needs from the editor around it: the lists it chooses from, and what it may do to itself. */
export interface WorkflowUi {
	/** "edit" while a workflow is drawn, "run" while one is shown read-only */
	mode: 'edit' | 'run';
	campaigns: Campaign[];
	labels: InboxLabel[];
	/** the personal Zalo accounts a send block may send through */
	zaloAccounts: ZaloAccount[];
	folders: string[];
	sheet: SheetInfo | null;
	setSheet: (sheet: SheetInfo | null) => void;
	/** the block whose settings are open next to it */
	panelId: string | null;
	startExists: boolean;
	update: (id: string, patch: Partial<NodeData>) => void;
	remove: (id: string) => void;
	duplicate: (id: string) => void;
	/** open the menu that adds a block after an output that has no wire */
	quickAdd: (id: string, port: WorkflowPort, at: { x: number; y: number }) => void;
	openPanel: (id: string) => void;
	closePanel: () => void;
}

export const WorkflowUiContext = createContext<WorkflowUi | null>(null);

export function useWorkflowUi(): WorkflowUi {
	const ui = useContext(WorkflowUiContext);
	if (!ui) throw new Error('a workflow block is drawn outside of its editor');
	return ui;
}

/** The context of a canvas that only shows: nothing can be changed. */
export const READ_ONLY_UI: WorkflowUi = {
	mode: 'run', campaigns: [], labels: [], zaloAccounts: [], folders: [], sheet: null, setSheet: () => undefined, panelId: null, startExists: true,
	update: () => undefined, remove: () => undefined, duplicate: () => undefined, quickAdd: () => undefined, openPanel: () => undefined, closePanel: () => undefined,
};
