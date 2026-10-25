import type { ButtonHTMLAttributes } from 'react'

const variants = {
  primary:
    'bg-indigo-600 text-white hover:bg-indigo-500 disabled:bg-indigo-300',
  secondary:
    'border border-slate-300 bg-white text-slate-700 hover:bg-slate-50 disabled:text-slate-400',
  danger: 'bg-white text-red-600 border border-red-200 hover:bg-red-50',
  ghost: 'text-slate-600 hover:bg-slate-200/60 hover:text-slate-900',
}

interface Props extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: keyof typeof variants
}

export function Button({
  variant = 'secondary',
  className = '',
  type = 'button',
  ...props
}: Props) {
  return (
    <button
      type={type}
      className={`inline-flex shrink-0 items-center justify-center gap-1 rounded-md whitespace-nowrap px-3 py-1.5 text-sm font-medium transition-colors disabled:cursor-not-allowed ${variants[variant]} ${className}`}
      {...props}
    />
  )
}
