import {
  Background,
  Controls,
  ReactFlow,
  type Connection,
  type Edge,
  type EdgeChange,
  type IsValidConnection,
  type NodeChange,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'

import type { StepNode } from './graph'
import { StepNodeView } from './StepNodeView'

const nodeTypes = { step: StepNodeView }

export interface FlowCanvasProps {
  nodes: StepNode[]
  edges: Edge[]
  onNodesChange: (changes: NodeChange<StepNode>[]) => void
  onEdgesChange: (changes: EdgeChange[]) => void
  onConnect: (connection: Connection) => void
  isValidConnection: IsValidConnection
}

/** The React Flow canvas; all state lives in the editor. */
export function FlowCanvas(props: FlowCanvasProps) {
  return (
    <ReactFlow
      {...props}
      nodeTypes={nodeTypes}
      deleteKeyCode={['Backspace', 'Delete']}
      fitView
      fitViewOptions={{ maxZoom: 1 }}
      proOptions={{ hideAttribution: true }}
    >
      <Background />
      <Controls showInteractive={false} />
    </ReactFlow>
  )
}
