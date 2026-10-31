import { useState } from 'react'
import Markdown from 'react-markdown'
import remarkGfm from 'remark-gfm'

import type { NodeRun } from '../api/client'
import { useDecideApproval } from '../api/queries'
import { Button } from '../components/Button'
import { ErrorMessage } from '../components/ErrorMessage'
import { TextArea } from '../components/Field'

interface Props {
  node: NodeRun
  className?: string
}

/**
 * An output awaiting approval: what the node before delivered, and the
 * human's answer. Asking for changes sends the message to the agent; the
 * flow comes back here with its new answer.
 */
export function ApprovalPanel({ node, className = '' }: Props) {
  const decide = useDecideApproval()
  const [message, setMessage] = useState('')
  return (
    <section
      aria-label="Aprovação"
      className={`flex min-h-0 flex-col gap-3 ${className}`}
    >
      <div className="markdown min-h-0 flex-1 overflow-y-auto rounded-md border border-slate-200 bg-white p-4">
        {node.input_message ? (
          <Markdown remarkPlugins={[remarkGfm]}>{node.input_message}</Markdown>
        ) : (
          <p className="text-sm text-slate-500">
            O nó anterior não entregou texto.
          </p>
        )}
      </div>
      <TextArea
        label="Ajustes"
        rows={3}
        value={message}
        onChange={setMessage}
      />
      <ErrorMessage error={decide.error} />
      <div className="flex justify-end gap-2">
        <Button
          disabled={decide.isPending || message.trim() === ''}
          onClick={() => {
            decide.mutate(
              { id: node.id, decision: 'changes_requested', message },
              {
                onSuccess: () => {
                  setMessage('')
                },
              },
            )
          }}
        >
          Pedir ajustes
        </Button>
        <Button
          variant="primary"
          disabled={decide.isPending}
          onClick={() => {
            decide.mutate({ id: node.id, decision: 'approved' })
          }}
        >
          Aprovar
        </Button>
      </div>
    </section>
  )
}
