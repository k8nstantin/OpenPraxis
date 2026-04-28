import { useRef } from 'react'

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
  onChange,
}: {
  label?: string
  value: number
  min: number
  max: number
  step?: number
  unit?: string
  defaultValue?: number
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
    <div className='bg-card flex flex-col items-center gap-0.5 rounded-md border px-1 py-2'>
      <svg
        ref={svgRef}
        viewBox='0 0 100 60'
        className={`w-full ${interactive ? 'cursor-pointer touch-none select-none' : ''}`}
        aria-label={label ?? ''}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
      >
        <path
          d={`M ${cx - R} ${cy} A ${R} ${R} 0 0 1 ${cx + R} ${cy}`}
          stroke='currentColor'
          strokeOpacity={0.15}
          strokeWidth={6}
          strokeLinecap='round'
          fill='none'
        />
        <path
          d={`M ${cx - R} ${cy} A ${R} ${R} 0 0 1 ${cx + R} ${cy}`}
          stroke='currentColor'
          strokeOpacity={0.7}
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
          strokeWidth={1.5}
          strokeLinecap='round'
        />
        <circle cx={cx} cy={cy} r={2.5} fill='currentColor' />
        {defaultValue !== undefined && range > 0 ? (() => {
          const df = Math.max(0, Math.min(1, (defaultValue - min) / range))
          const dt = -Math.PI / 2 + Math.PI * df
          const innerR = R - 5
          const outerR = R + 5
          const x1 = cx + innerR * Math.sin(dt)
          const y1 = cy - innerR * Math.cos(dt)
          const x2 = cx + outerR * Math.sin(dt)
          const y2 = cy - outerR * Math.cos(dt)
          // Default tick — always green so the operator can see at a
          // glance how far the needle has been dialed off the catalog
          // default. Independent of the surrounding `currentColor`
          // (which the deviation tone hijacks).
          return (
            <line
              x1={x1}
              y1={y1}
              x2={x2}
              y2={y2}
              stroke='#10b981'
              strokeWidth={1.5}
              strokeLinecap='round'
            />
          )
        })() : null}
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
      </svg>
      <span className='font-mono text-xs font-semibold'>
        {value}
        {unit ? (
          <span className='text-muted-foreground ml-0.5'>{unit}</span>
        ) : null}
      </span>
      {label ? (
        <span className='text-muted-foreground text-[9px] uppercase tracking-wider'>
          {label}
        </span>
      ) : null}
    </div>
  )
}
