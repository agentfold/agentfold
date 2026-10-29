import { useId, type ReactNode } from 'react'

import { palette } from './palette'

const inputClass =
  'w-full rounded-md border border-slate-300 bg-white px-2.5 py-1.5 text-sm shadow-xs focus:border-indigo-500 focus:outline-none focus:ring-2 focus:ring-indigo-500/20'

interface FieldProps {
  label: string
  hint?: string
  children: (id: string) => ReactNode
}

function Field({ label, hint, children }: FieldProps) {
  const id = useId()
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={id} className="text-xs font-medium text-slate-600">
        {label}
      </label>
      {children(id)}
      {hint && <p className="text-xs text-slate-500">{hint}</p>}
    </div>
  )
}

interface TextFieldProps {
  label: string
  value: string
  onChange: (value: string) => void
  hint?: string
  placeholder?: string
  autoFocus?: boolean
  type?: 'text' | 'number'
}

export function TextField({
  label,
  value,
  onChange,
  hint,
  placeholder,
  autoFocus,
  type = 'text',
}: TextFieldProps) {
  return (
    <Field label={label} hint={hint}>
      {(id) => (
        <input
          id={id}
          type={type}
          className={inputClass}
          value={value}
          placeholder={placeholder}
          autoFocus={autoFocus}
          onChange={(e) => {
            onChange(e.target.value)
          }}
        />
      )}
    </Field>
  )
}

interface TextAreaProps {
  label: string
  value: string
  onChange: (value: string) => void
  hint?: string
  rows?: number
  mono?: boolean
  disabled?: boolean
}

export function TextArea({
  label,
  value,
  onChange,
  hint,
  rows = 4,
  mono = false,
  disabled = false,
}: TextAreaProps) {
  return (
    <Field label={label} hint={hint}>
      {(id) => (
        <textarea
          id={id}
          rows={rows}
          className={`${inputClass} ${mono ? 'font-mono text-xs' : ''} disabled:bg-slate-50`}
          value={value}
          disabled={disabled}
          onChange={(e) => {
            onChange(e.target.value)
          }}
        />
      )}
    </Field>
  )
}

export interface Option {
  value: string
  label: string
}

interface SelectFieldProps {
  label: string
  value: string
  options: Option[]
  onChange: (value: string) => void
}

export function SelectField({
  label,
  value,
  options,
  onChange,
}: SelectFieldProps) {
  return (
    <Field label={label}>
      {(id) => (
        <select
          id={id}
          className={inputClass}
          value={value}
          onChange={(e) => {
            onChange(e.target.value)
          }}
        >
          {options.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      )}
    </Field>
  )
}

interface ColorFieldProps {
  value: string
  onChange: (value: string) => void
}

/** Picks one color of the palette, or none. */
export function ColorField({ value, onChange }: ColorFieldProps) {
  return (
    <fieldset className="flex flex-col gap-1">
      <legend className="text-xs font-medium text-slate-600">Cor</legend>
      <div className="flex flex-wrap items-center gap-1.5 pt-1">
        <Swatch
          label="Sem cor"
          color=""
          selected={value === ''}
          onSelect={onChange}
        />
        {palette.map((color) => (
          <Swatch
            key={color}
            label={color}
            color={color}
            selected={value === color}
            onSelect={onChange}
          />
        ))}
      </div>
    </fieldset>
  )
}

interface SwatchProps {
  label: string
  color: string
  selected: boolean
  onSelect: (color: string) => void
}

function Swatch({ label, color, selected, onSelect }: SwatchProps) {
  return (
    <button
      type="button"
      aria-label={label}
      aria-pressed={selected}
      onClick={() => {
        onSelect(color)
      }}
      className={`size-6 rounded-full border ${selected ? 'ring-2 ring-indigo-500 ring-offset-1' : ''} ${color ? 'border-transparent' : 'border-dashed border-slate-400 bg-white'}`}
      style={color ? { backgroundColor: color } : undefined}
    />
  )
}
