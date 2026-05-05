// Shared chart utilities — time formatting, ET timezone, data conversion.

const ET_ZONE = 'America/New_York'

// Format a ms timestamp as a short date or time in Eastern time.
// ECharts time axis passes ms to formatter functions.
export function fmtET(ms: number, hourly = false): string {
  if (hourly) {
    return new Intl.DateTimeFormat('en-US', {
      timeZone: ET_ZONE,
      hour: 'numeric',
      hour12: false,
    }).format(new Date(ms)) + 'h'
  }
  return new Intl.DateTimeFormat('en-US', {
    timeZone: ET_ZONE,
    month: '2-digit',
    day: '2-digit',
  }).format(new Date(ms))
}

// Convert a "2026-05-05T09:00:00Z" or "2026-05-05" string to ms timestamp.
export function toMs(iso: string): number {
  return new Date(iso.length === 10 ? iso + 'T00:00:00Z' : iso).getTime()
}

// ECharts xAxis config for time series (fixes all boundary/alignment issues).
export function timeXAxis(hourly = false) {
  return {
    type: 'time' as const,
    boundaryGap: false,
    axisLabel: {
      fontSize: 9,
      formatter: (ms: number) => fmtET(ms, hourly),
    },
  }
}

// Standard tooltip position: flip left when near right edge.
export const tooltipPosition = (
  point: number[],
  _p: unknown, _d: unknown, _r: unknown,
  size: { contentSize: number[]; viewSize: number[] }
) => {
  const [x] = point
  const [w] = size.contentSize
  const [vw] = size.viewSize
  return [x > vw / 2 ? x - w - 20 : x + 20, 12]
}

export const RANGES = [
  { label: '1d', days: 1 },
  { label: '2d', days: 2 },
  { label: '3d', days: 3 },
  { label: '1w', days: 7 },
  { label: '2w', days: 14 },
  { label: '1m', days: 30 },
  { label: '3m', days: 90 },
  { label: 'All', days: 0 },
] as const

export type RangeDays = typeof RANGES[number]['days']
