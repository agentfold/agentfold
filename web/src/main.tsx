import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'

import App from './App.tsx'
import { createApiClient } from './api/client'
import { ApiContext } from './api/context'
import './index.css'

const root = document.getElementById('root')
if (!root) {
  throw new Error('missing #root element')
}

const api = {
  client: createApiClient(`${window.location.origin}/api`),
  openEvents: () => new EventSource('/api/events'),
}

createRoot(root).render(
  <StrictMode>
    <QueryClientProvider client={new QueryClient()}>
      <ApiContext value={api}>
        <App />
      </ApiContext>
    </QueryClientProvider>
  </StrictMode>,
)
