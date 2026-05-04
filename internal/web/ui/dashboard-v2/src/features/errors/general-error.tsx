import { useNavigate, useRouter } from '@tanstack/react-router'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'

type GeneralErrorProps = React.HTMLAttributes<HTMLDivElement> & {
  minimal?: boolean
  error?: unknown
  reset?: () => void
}

export function GeneralError({
  className,
  minimal = false,
  error,
}: GeneralErrorProps) {
  const errMsg = error instanceof Error ? error.message : error ? String(error) : undefined
  const stack = error instanceof Error ? error.stack : undefined
  const navigate = useNavigate()
  const { history } = useRouter()
  return (
    <div className={cn('h-svh w-full', className)}>
      <div className='m-auto flex h-full w-full flex-col items-center justify-center gap-2'>
        {!minimal && (
          <h1 className='text-[7rem] leading-tight font-bold'>500</h1>
        )}
        <span className='font-medium'>Oops! Something went wrong {`:')`}</span>
        {errMsg && (
          <pre className='max-w-2xl overflow-auto rounded bg-zinc-900 p-3 text-left text-xs text-rose-400 whitespace-pre-wrap'>
            {errMsg}{'\n\n'}{stack}
          </pre>
        )}
        <p className='text-center text-muted-foreground'>
          We apologize for the inconvenience. <br /> Please try again later.
        </p>
        {!minimal && (
          <div className='mt-6 flex gap-4'>
            <Button variant='outline' onClick={() => history.go(-1)}>
              Go Back
            </Button>
            <Button onClick={() => navigate({ to: '/' })}>Back to Home</Button>
          </div>
        )}
      </div>
    </div>
  )
}
