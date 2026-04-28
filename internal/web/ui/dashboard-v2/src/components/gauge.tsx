// Speedometer-style gauge: semi-circle from min (left) to max (right),
// needle at the current value. Pure SVG; no library. Used both as a
// read-only visualization on Main and alongside the number input on
// Execution Control.
export function Gauge({
  label,
  value,
  min,
  max,
  unit,
  size = 'md',
}: {
  label?: string
  value: number
  min: number
  max: number
  unit?: string
  size?: 'sm' | 'md'
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
  const wrapper =
    size === 'sm'
      ? 'flex flex-col items-center gap-0 px-1'
      : 'bg-card flex flex-col items-center gap-0.5 rounded-md border px-1 py-2'
  return (
    <div className={wrapper}>
      <svg viewBox='0 0 100 60' className='w-full' aria-label={label ?? ''}>
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
      {size === 'md' ? (
        <>
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
        </>
      ) : null}
    </div>
  )
}
