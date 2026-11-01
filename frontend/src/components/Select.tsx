import { Children, type ReactElement, type ReactNode, isValidElement, useCallback, useEffect, useId, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';

interface Props {
	value?: string;
	defaultValue?: string;
	name?: string;
	className?: string;
	disabled?: boolean;
	required?: boolean;
	onChange?: (event: any) => void;
	children?: ReactNode;
}

interface Item {
	value: string;
	label: ReactNode;
	disabled: boolean;
}

/**
 * Drop-in replacement for <select>: same props/children (<option>), but with an animated, keyboard-accessible
 * menu rendered in a portal (so it is never clipped by modals). onChange receives { target: { value, name } }.
 */
export default function Select({ value, defaultValue, name, className = '', disabled, onChange, children }: Props) {
	const items = useMemo<Item[]>(
		() =>
			Children.toArray(children)
				.filter((c): c is ReactElement<any> => isValidElement(c) && (c as ReactElement<any>).type === 'option')
				.map((c) => ({ value: String(c.props.value ?? ''), label: c.props.children, disabled: Boolean(c.props.disabled) })),
		[children],
	);
	const [inner, setInner] = useState(defaultValue ?? items[0]?.value ?? '');
	const current = value ?? inner;
	const selected = items.find((i) => i.value === current) ?? items[0];

	const [open, setOpen] = useState(false);
	const [active, setActive] = useState(0);
	const [rect, setRect] = useState<{ top: number; left: number; width: number; up: boolean } | null>(null);
	const trigger = useRef<HTMLButtonElement>(null);
	const menu = useRef<HTMLUListElement>(null);
	const id = useId();

	const place = useCallback(() => {
		const r = trigger.current?.getBoundingClientRect();
		if (!r) return;
		const need = Math.min(items.length * 38 + 12, 280);
		const up = window.innerHeight - r.bottom < need && r.top > need;
		setRect({ top: up ? r.top - 6 : r.bottom + 6, left: r.left, width: Math.max(r.width, 160), up });
	}, [items.length]);

	function openMenu() {
		if (disabled) return;
		place();
		setActive(Math.max(0, items.findIndex((i) => i.value === current)));
		setOpen(true);
	}

	function choose(item: Item) {
		if (item.disabled) return;
		setInner(item.value);
		onChange?.({ target: { value: item.value, name } });
		setOpen(false);
		trigger.current?.focus();
	}

	useEffect(() => {
		if (!open) return undefined;
		const close = (e: Event) => {
			if (e.type === 'scroll' && menu.current?.contains(e.target as Node)) return;
			setOpen(false);
		};
		const down = (e: MouseEvent) => {
			if (!menu.current?.contains(e.target as Node) && !trigger.current?.contains(e.target as Node)) setOpen(false);
		};
		document.addEventListener('mousedown', down);
		window.addEventListener('resize', close);
		window.addEventListener('scroll', close, true);
		return () => {
			document.removeEventListener('mousedown', down);
			window.removeEventListener('resize', close);
			window.removeEventListener('scroll', close, true);
		};
	}, [open]);

	useEffect(() => {
		if (open) menu.current?.querySelector<HTMLElement>('[data-active="true"]')?.scrollIntoView({ block: 'nearest' });
	}, [open, active]);

	function onKeyDown(e: React.KeyboardEvent) {
		if (disabled) return;
		if (!open) {
			if (['ArrowDown', 'ArrowUp', 'Enter', ' '].includes(e.key)) {
				e.preventDefault();
				openMenu();
			}
			return;
		}
		if (e.key === 'Escape') {
			e.stopPropagation();
			setOpen(false);
		} else if (e.key === 'ArrowDown') {
			e.preventDefault();
			setActive((a) => Math.min(items.length - 1, a + 1));
		} else if (e.key === 'ArrowUp') {
			e.preventDefault();
			setActive((a) => Math.max(0, a - 1));
		} else if (e.key === 'Enter' || e.key === ' ') {
			e.preventDefault();
			choose(items[active]);
		} else if (e.key === 'Tab') {
			setOpen(false);
		}
	}

	return (
		<div className={`ui-select ${className}`}>
			<button
				ref={trigger}
				type="button"
				className={`ui-sel-trigger${open ? ' open' : ''}`}
				disabled={disabled}
				aria-haspopup="listbox"
				aria-expanded={open}
				aria-controls={id}
				onClick={() => (open ? setOpen(false) : openMenu())}
				onKeyDown={onKeyDown}
			>
				<span className="ui-sel-label">{selected?.label}</span>
				<svg className="ui-sel-chevron" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round">
					<polyline points="6 9 12 15 18 9" />
				</svg>
			</button>
			{open &&
				rect &&
				createPortal(
					<ul
						ref={menu}
						id={id}
						role="listbox"
						className={`ui-sel-menu${rect.up ? ' up' : ''}`}
						style={{ left: rect.left, minWidth: rect.width, ...(rect.up ? { bottom: window.innerHeight - rect.top } : { top: rect.top }) }}
					>
						{items.map((item, index) => (
							<li
								key={item.value}
								role="option"
								aria-selected={item.value === current}
								aria-disabled={item.disabled}
								data-active={index === active}
								className={`ui-sel-option${item.value === current ? ' selected' : ''}${index === active ? ' active' : ''}${item.disabled ? ' disabled' : ''}`}
								onMouseEnter={() => setActive(index)}
								onMouseDown={(e) => e.preventDefault()}
								onClick={() => choose(item)}
							>
								<span>{item.label}</span>
								{item.value === current && (
									<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.6" strokeLinecap="round" strokeLinejoin="round">
										<polyline points="20 6 9 17 4 12" />
									</svg>
								)}
							</li>
						))}
					</ul>,
					document.body,
				)}
		</div>
	);
}
