import { useEffect, useMemo, useRef, useState } from 'react'
import ReactECharts from 'echarts-for-react'
import type { EChartsOption } from 'echarts'

// Thin wrapper around echarts-for-react that themes via the dashboard's
// CSS vars. Reads --card / --foreground / --muted-foreground / --border
// at render time and feeds them into ECharts' textStyle / axisLine /
// splitLine colors so charts visually match the rest of the UI when the
// theme switches.
//
// Defaults: notMerge:true, lazyUpdate:true. Caller-supplied option is
// merged on top so any series-level override wins.

interface EChartProps {
  option: EChartsOption
  height?: number | string
  className?: string
  // notMerge=false preserves existing chart state (e.g. force-layout node
  // positions) on option updates. Defaults to true (full replace).
  notMerge?: boolean
  onEvents?: Record<string, (...args: unknown[]) => void>
}

function readVar(name: string): string {
  if (typeof window === 'undefined') return ''
  return (
    getComputedStyle(document.documentElement).getPropertyValue(name).trim() ||
    ''
  )
}

export function EChart({
  option,
  height = 280,
  className,
  notMerge = true,
  onEvents,
}: EChartProps) {
  const chartRef = useRef<ReactECharts>(null)
  // Re-render on theme changes by tracking a token derived from the
  // CSS vars. Cheap — runs only when the system theme flips.
  const [themeToken, setThemeToken] = useState(0)
  useEffect(() => {
    if (typeof window === 'undefined') return
    const obs = new MutationObserver(() => setThemeToken((t) => t + 1))
    obs.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['class', 'style', 'data-theme'],
    })
    return () => obs.disconnect()
  }, [])

  const themed = useMemo<EChartsOption>(() => {
    const fg = readVar('--foreground') || '#e5e7eb'
    const muted = readVar('--muted-foreground') || '#94a3b8'
    const border = readVar('--border') || '#1f2937'
    const card = readVar('--card') || 'transparent'

    return {
      ...option,
      backgroundColor: option.backgroundColor ?? card,
      textStyle: {
        color: fg,
        ...((option.textStyle as object) ?? {}),
      },
      grid: {
        left: 48,
        right: 24,
        top: 32,
        bottom: 36,
        containLabel: true,
        ...((option.grid as object) ?? {}),
      },
      tooltip: {
        backgroundColor: card,
        borderColor: border,
        textStyle: { color: fg },
        ...((option.tooltip as object) ?? {}),
      },
      legend: option.legend
        ? {
            textStyle: { color: muted },
            ...(option.legend as object),
          }
        : undefined,
      xAxis: stamp(option.xAxis, fg, muted, border),
      yAxis: stamp(option.yAxis, fg, muted, border),
    }
  }, [option, themeToken])

  // ResizeObserver on the wrapper — force chart.resize() on every
  // container layout change. Without this ECharts can lock to a width
  // computed at first paint, then overflow when the grid gives it
  // less / more space later.
  const wrapRef = useRef<HTMLDivElement | null>(null)
  useEffect(() => {
    const wrap = wrapRef.current
    if (!wrap) return
    const ro = new ResizeObserver(() => {
      const inst = chartRef.current?.getEchartsInstance?.()
      if (inst) inst.resize()
    })
    ro.observe(wrap)
    return () => ro.disconnect()
  }, [])

  // When the caller passes height='100%' the wrapper must forward it
  // — without an explicit height the inner ReactECharts collapses to
  // its content (often ~100px) and the chart renders into a tiny
  // strip while the parent has hundreds of px of vertical space.
  const wrapHeight =
    typeof height === 'string' ? height : `${height}px`
  return (
    <div ref={wrapRef} style={{ width: '100%', height: wrapHeight, overflow: 'hidden' }}>
      <ReactECharts
        ref={chartRef}
        option={themed}
        style={{ height: '100%', width: '100%' }}
        className={className}
        notMerge={notMerge}
        lazyUpdate
        opts={{ renderer: 'canvas' }}
        onEvents={onEvents}
      />
    </div>
  )
}

// stamp augments an axis spec with themed colors. Accepts a single axis
// or an array of axes (ECharts supports both for layered grids).
function stamp(
  axis: unknown,
  fg: string,
  muted: string,
  border: string
): unknown {
  if (!axis) return axis
  if (Array.isArray(axis)) return axis.map((a) => stamp(a, fg, muted, border))
  const a = axis as Record<string, unknown>
  return {
    ...a,
    axisLine: {
      lineStyle: { color: border },
      ...((a.axisLine as object) ?? {}),
    },
    axisLabel: {
      color: muted,
      ...((a.axisLabel as object) ?? {}),
    },
    splitLine: {
      lineStyle: { color: border, opacity: 0.4 },
      ...((a.splitLine as object) ?? {}),
    },
    nameTextStyle: {
      color: fg,
      ...((a.nameTextStyle as object) ?? {}),
    },
  }
}
