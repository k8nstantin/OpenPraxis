import { useId, useRef } from 'react'

// Compact value formatting for the needle-tip badge — keeps the
// label inside the 12px disc. 1234 → 1.2k, 1.234M → 1.2M, 0.123 → .12.
function fmtTip(v: number): string {
  if (!Number.isFinite(v)) return '—'
  const abs = Math.abs(v)
  if (abs >= 1_000_000) return (v / 1_000_000).toFixed(1) + 'M'
  if (abs >= 1_000) return (v / 1_000).toFixed(1) + 'k'
  if (Number.isInteger(v)) return String(v)
  if (abs >= 10) return v.toFixed(0)
  if (abs >= 1) return v.toFixed(1)
  return v.toFixed(2)
}

// Tick mark crossing the arc at the fraction (value-min)/range, painted
// in `color` (hard-coded so deviation tone on the wrapper doesn't
// repaint it). Used for both the default tick (green) on Execution
// Control and the budget red-line on the Main cost gauges.
function renderTick(
  cx: number,
  cy: number,
  R: number,
  min: number,
  range: number,
  v: number,
  color: string
) {
  const f = Math.max(0, Math.min(1, (v - min) / range))
  const t = -Math.PI / 2 + Math.PI * f
  const innerR = R - 5
  const outerR = R + 5
  const x1 = cx + innerR * Math.sin(t)
  const y1 = cy - innerR * Math.cos(t)
  const x2 = cx + outerR * Math.sin(t)
  const y2 = cy - outerR * Math.cos(t)
  return (
    <line
      x1={x1}
      y1={y1}
      x2={x2}
      y2={y2}
      stroke={color}
      strokeWidth={1.5}
      strokeLinecap='round'
    />
  )
}

// Speedometer-style gauge: semi-circle from min (left) to max (right),
// needle at the current value. Pure SVG; no library. Read-only when
// `onChange` is omitted; interactive (click + drag the arc to set the
// value) when `onChange` is provided.
export function Gauge({
  label,
  value,
  min,
  max,
  step,
  unit,
  defaultValue,
  redLine,
  onChange,
}: {
  label?: string
  value: number
  min: number
  max: number
  step?: number
  unit?: string
  defaultValue?: number
  redLine?: number
  onChange?: (next: number) => void
}) {
  const range = max - min
  const f = range > 0 ? Math.max(0, Math.min(1, (value - min) / range)) : 0
  const cx = 50
  const cy = 48
  const R = 36
  // Map fraction f∈[0,1] → angle θ∈[-π/2, π/2] (left → up → right).
  const theta = -Math.PI / 2 + Math.PI * f
  const nx = cx + R * Math.sin(theta)
  const ny = cy - R * Math.cos(theta)
  const arcLen = Math.PI * R
  const interactive = !!onChange
  const svgRef = useRef<SVGSVGElement | null>(null)
  const gradId = `gauge-grad-${useId().replace(/:/g, '')}`

  const updateFromPointer = (clientX: number, clientY: number) => {
    if (!svgRef.current || !onChange) return
    const rect = svgRef.current.getBoundingClientRect()
    // Map screen coords back to SVG viewBox space (0..100 × 0..60).
    const px = ((clientX - rect.left) / rect.width) * 100
    const py = ((clientY - rect.top) / rect.height) * 60
    const dx = px - cx
    const dy = cy - py // invert: screen-y down → math-y up
    // atan2(dx, dy) gives 0 at top (12 o'clock), -π/2 left, +π/2 right.
    let t = Math.atan2(dx, dy)
    // Clamp to the upper semi-circle.
    t = Math.max(-Math.PI / 2, Math.min(Math.PI / 2, t))
    const nf = (t + Math.PI / 2) / Math.PI
    let next = min + nf * range
    const s = step && step > 0 ? step : (Number.isInteger(min) && Number.isInteger(max) ? 1 : 0.01)
    next = Math.round(next / s) * s
    if (next < min) next = min
    if (next > max) next = max
    // Round to a sensible precision so float math doesn't leak 0.30000000000000004.
    if (s < 1) next = parseFloat(next.toFixed(4))
    onChange(next)
  }

  const onPointerDown = (e: React.PointerEvent<SVGSVGElement>) => {
    if (!interactive) return
    e.preventDefault()
    e.currentTarget.setPointerCapture(e.pointerId)
    updateFromPointer(e.clientX, e.clientY)
  }
  const onPointerMove = (e: React.PointerEvent<SVGSVGElement>) => {
    if (!interactive) return
    if (!(e.buttons & 1)) return
    updateFromPointer(e.clientX, e.clientY)
  }

  return (
    <div className='bg-card rounded-md border'>
      <svg
        ref={svgRef}
        viewBox='0 0 100 90'
        className={`block w-full ${interactive ? 'cursor-pointer touch-none select-none' : ''}`}
        aria-label={label ?? ''}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
      >
        <defs>
          <linearGradient id={gradId} x1='0%' y1='0%' x2='100%' y2='0%'>
            <stop offset='0%' stopColor='#10b981' stopOpacity={0.5} />
            <stop offset='66%' stopColor='#f59e0b' stopOpacity={0.5} />
            <stop offset='100%' stopColor='#ef4444' stopOpacity={0.6} />
          </linearGradient>
        </defs>
        {/* Background arc — left-to-right green→amber→red gradient
            so the operator can see the value-zone progression at a
            glance. The gradient is along the SVG x-axis, which on a
            semicircle maps cleanly to the arc's left-to-right sweep. */}
        <path
          d={`M ${cx - R} ${cy} A ${R} ${R} 0 0 1 ${cx + R} ${cy}`}
          stroke={`url(#${gradId})`}
          strokeWidth={6}
          strokeLinecap='round'
          fill='none'
        />
        {/* Filled arc up to the current value — deviation tone via
            currentColor; sits on top of the gradient bg. */}
        <path
          d={`M ${cx - R} ${cy} A ${R} ${R} 0 0 1 ${cx + R} ${cy}`}
          stroke='currentColor'
          strokeOpacity={0.95}
          strokeWidth={6}
          strokeLinecap='round'
          fill='none'
          strokeDasharray={arcLen}
          strokeDashoffset={arcLen * (1 - f)}
        />
        <line
          x1={cx}
          y1={cy}
          x2={nx}
          y2={ny}
          stroke='currentColor'
          strokeWidth={2.5}
          strokeLinecap='round'
        />
        <circle cx={cx} cy={cy} r={3} fill='currentColor' />
        {/* Number at the needle tip — small disc with the value
            stamped on it so the operator can read position even at
            tight grid sizes. Placed slightly outside the arc so it
            doesn't overlap the needle line. */}
        <circle
          cx={cx + (R + 6) * Math.sin(theta)}
          cy={cy - (R + 6) * Math.cos(theta)}
          r={6}
          fill='currentColor'
        />
        <text
          x={cx + (R + 6) * Math.sin(theta)}
          y={cy - (R + 6) * Math.cos(theta) + 2}
          textAnchor='middle'
          fontSize='6'
          fontWeight='bold'
          fill='white'
        >
          {fmtTip(value)}
        </text>
        {/* Minor notches at 25/50/75% so the operator can read scale
            position without staring — like a real speedometer. Drawn
            before the value ticks so default + red-line stay on top. */}
        {[0.25, 0.5, 0.75].map((nf) => {
          const t = -Math.PI / 2 + Math.PI * nf
          const innerR = R - 2
          const outerR = R + 2
          const x1 = cx + innerR * Math.sin(t)
          const y1 = cy - innerR * Math.cos(t)
          const x2 = cx + outerR * Math.sin(t)
          const y2 = cy - outerR * Math.cos(t)
          return (
            <line
              key={nf}
              x1={x1}
              y1={y1}
              x2={x2}
              y2={y2}
              stroke='currentColor'
              strokeOpacity={0.25}
              strokeWidth={0.75}
              strokeLinecap='round'
            />
          )
        })}
        {defaultValue !== undefined && range > 0
          ? renderTick(cx, cy, R, min, range, defaultValue, '#10b981')
          : null}
        {redLine !== undefined && range > 0
          ? renderTick(cx, cy, R, min, range, redLine, '#ef4444')
          : null}
        <text
          x={cx - R}
          y={58}
          textAnchor='start'
          fontSize='6'
          fill='currentColor'
          opacity={0.5}
        >
          {min}
        </text>
        <text
          x={cx + R}
          y={58}
          textAnchor='end'
          fontSize='6'
          fill='currentColor'
          opacity={0.5}
        >
          {max}
        </text>
        {/* Value + label live inside the SVG so they scale with the
            gauge as the container resizes (text-xs CSS sizing didn't
            track the SVG). Two lines: bold value (with unit) at y=72,
            uppercase label at y=84. */}
        <text
          x={50}
          y={72}
          textAnchor='middle'
          fontSize='10'
          fontWeight='bold'
          fill='currentColor'
          fontFamily='ui-monospace, SFMono-Regular, Menlo, monospace'
        >
          {String(value)}
          {unit ? ` ${unit}` : ''}
        </text>
        {label ? (
          <text
            x={50}
            y={84}
            textAnchor='middle'
            fontSize='6'
            fill='currentColor'
            opacity={0.6}
            letterSpacing='0.5'
            style={{ textTransform: 'uppercase' }}
          >
            {label}
          </text>
        ) : null}
      </svg>
    </div>
  )
}
