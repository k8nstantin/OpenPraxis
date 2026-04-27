// Drag-to-resize hook used by the Products sidebar (#245). Persists the
// last width to localStorage so the operator's choice survives reloads.
// Constraints: clamped to [minPx, maxPctOfViewport * window.innerWidth]
// to keep the detail pane usable.
import { useCallback, useEffect, useRef, useState } from 'react';

export interface UseResizableArgs {
  storageKey: string;
  defaultPx: number;
  minPx?: number;
  maxPct?: number;
}

export function useResizable({ storageKey, defaultPx, minPx = 200, maxPct = 0.6 }: UseResizableArgs) {
  const [widthPx, setWidthPx] = useState<number>(() => {
    try {
      const v = window.localStorage.getItem(storageKey);
      const n = v ? Number(v) : NaN;
      if (Number.isFinite(n) && n > 0) return n;
    } catch {
      // localStorage may be denied — fall back to default.
    }
    return defaultPx;
  });
  const dragging = useRef(false);

  useEffect(() => {
    try {
      window.localStorage.setItem(storageKey, String(widthPx));
    } catch {
      // ignore quota / privacy-mode failures
    }
  }, [storageKey, widthPx]);

  const onPointerDown = useCallback(() => {
    dragging.current = true;
    document.body.style.cursor = 'col-resize';
    document.body.style.userSelect = 'none';
  }, []);

  useEffect(() => {
    const onMove = (e: PointerEvent) => {
      if (!dragging.current) return;
      const max = Math.max(minPx + 50, window.innerWidth * maxPct);
      const next = Math.min(Math.max(e.clientX, minPx), max);
      setWidthPx(next);
    };
    const onUp = () => {
      if (!dragging.current) return;
      dragging.current = false;
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
    };
    window.addEventListener('pointermove', onMove);
    window.addEventListener('pointerup', onUp);
    return () => {
      window.removeEventListener('pointermove', onMove);
      window.removeEventListener('pointerup', onUp);
    };
  }, [minPx, maxPct]);

  return { widthPx, onPointerDown };
}
