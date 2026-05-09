import { useState } from 'react'
import { Check, Copy } from 'lucide-react'

interface CopyButtonProps {
  text: string
  label?: string       // optional visible label; defaults to icon-only
  className?: string
  title?: string
}

export function CopyButton({ text, label, className = '', title = 'Copy' }: CopyButtonProps) {
  const [copied, setCopied] = useState(false)

  const handleCopy = (e: React.MouseEvent) => {
    e.stopPropagation()
    navigator.clipboard.writeText(text).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    })
  }

  return (
    <button
      type='button'
      onClick={handleCopy}
      title={title}
      className={`inline-flex items-center gap-1 text-muted-foreground hover:text-foreground transition-colors ${className}`}
    >
      {copied
        ? <Check className='h-3 w-3 text-emerald-400' />
        : <Copy className='h-3 w-3' />}
      {label && <span className='text-[10px]'>{copied ? 'Copied!' : label}</span>}
    </button>
  )
}
