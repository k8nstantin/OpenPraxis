// React port of legacy `OL.descToggle` (PR #239). Toggles between the
// raw markup source and the rendered HTML/markdown for a description
// field. Persisted via the Zustand UI store so the user's preference
// survives navigation and reloads.
import { useUiStore, type DescMode } from '@/lib/stores/ui';
import clsx from 'clsx';

export interface DescToggleProps {
  className?: string;
  size?: 'sm' | 'md';
}

export function DescToggle({ className, size = 'sm' }: DescToggleProps) {
  const mode = useUiStore((s) => s.descMode);
  const setMode = useUiStore((s) => s.setDescMode);
  return (
    <div
      role="radiogroup"
      aria-label="Description display mode"
      className={clsx('desc-toggle', `desc-toggle--${size}`, className)}
    >
      <ToggleBtn current={mode} value="markup" onChange={setMode}>Markup</ToggleBtn>
      <ToggleBtn current={mode} value="rendered" onChange={setMode}>Rendered</ToggleBtn>
    </div>
  );
}

function ToggleBtn({
  current,
  value,
  onChange,
  children,
}: {
  current: DescMode;
  value: DescMode;
  onChange: (m: DescMode) => void;
  children: string;
}) {
  const active = current === value;
  return (
    <button
      type="button"
      role="radio"
      aria-checked={active}
      className={clsx('desc-toggle__btn', active && 'is-active')}
      onClick={() => onChange(value)}
    >
      {children}
    </button>
  );
}
