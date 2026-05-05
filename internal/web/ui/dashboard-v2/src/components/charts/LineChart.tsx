/**
 * LineChart — generic single or multi-line time-series chart.
 * Uses xAxis: { type: 'time' } so:
 *   - No boundaryGap hacks needed
 *   - First point is flush with y-axis
 *   - Labels auto-format based on time span (dates for daily, hours for hourly)
 *   - y-axis 0 is always at the bottom baseline
 */
import { EChart } from '@/components/echart'
import { tooltipPosition } from './utils'

export interface LineSeries {
  name: string
  data: [number, number][] // [timestamp_ms, value]
  color: string
  area?: boolean           // fill area under curve
  yIndex?: number          // 0 = left axis (default), 1 = right axis
  unit?: string            // appended to tooltip value: "%" or "s" etc.
}

interface LineChartProps {
  series: LineSeries[]
  height?: number
  yLeft?: { min?: number; max?: number; unit?: string }
  yRight?: { min?: number; max?: number; unit?: string }
  showLegend?: boolean
}

const GRADIENT = (color: string) => ({
  type: 'linear' as const, x: 0, y: 0, x2: 0, y2: 1,
  colorStops: [{ offset: 0, color: color + '55' }, { offset: 1, color: color + '00' }],
})

export function LineChart({ series, height = 180, yLeft, yRight, showLegend }: LineChartProps) {
  const hasRight = series.some(s => s.yIndex === 1)

  const yAxes = [
    {
      type: 'value' as const,
      min: yLeft?.min ?? 0,
      ...(yLeft?.max !== undefined ? { max: yLeft.max } : {}),
      axisLabel: { fontSize: 9, formatter: yLeft?.unit ? (v: number) => `${v}${yLeft.unit}` : undefined },
      splitLine: { lineStyle: { opacity: 0.15 } },
    },
    ...(hasRight ? [{
      type: 'value' as const,
      min: yRight?.min ?? 0,
      ...(yRight?.max !== undefined ? { max: yRight.max } : {}),
      axisLabel: { fontSize: 9, formatter: yRight?.unit ? (v: number) => `${v}${yRight.unit}` : undefined },
      splitLine: { show: false },
      position: 'right' as const,
    }] : []),
  ]

  return (
    <EChart height={height} option={{
      grid: { left: 44, right: hasRight ? 56 : 16, top: 10, bottom: showLegend ? 40 : 28 },
      tooltip: {
        trigger: 'axis',
        confine: true,
        position: tooltipPosition,
      },
      ...(showLegend ? { legend: { bottom: 0, itemWidth: 8, itemHeight: 8, textStyle: { fontSize: 8 } } } : {}),
      xAxis: {
        type: 'time',
        boundaryGap: false,
        axisLabel: { fontSize: 9 },
      },
      yAxis: yAxes,
      series: series.map(s => ({
        name: s.name,
        type: 'line' as const,
        yAxisIndex: s.yIndex ?? 0,
        data: s.data,
        smooth: true,
        smoothMonotone: 'x',
        showSymbol: false,
        lineStyle: { color: s.color, width: 2 },
        ...(s.area ? { areaStyle: { color: GRADIENT(s.color) } } : {}),
      })),
    }} />
  )
}
