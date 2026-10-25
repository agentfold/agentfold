import { AgentsPage } from './agents/AgentsPage'
import { useBoardEvents } from './api/events'
import { Board } from './board/Board'
import { CardPage } from './cards/CardPage'
import { CardTypesPage } from './cardTypes/CardTypesPage'
import { FlowEditor } from './flows/FlowEditor'
import { FlowsPage } from './flows/FlowsPage'
import { ProjectsPage } from './projects/ProjectsPage'
import { pageHref, useRoute, type Page } from './route'

const tabs: { page: Page; label: string }[] = [
  { page: 'board', label: 'Board' },
  { page: 'projects', label: 'Projetos' },
  { page: 'flows', label: 'Fluxos' },
  { page: 'card-types', label: 'Tipos de card' },
  { page: 'agents', label: 'Agentes' },
]

function App() {
  const { page, id } = useRoute()
  useBoardEvents()
  // Card pages belong to the board; without an id they fall back to it.
  const current = page === 'cards' ? 'board' : page

  return (
    <div className="flex h-screen flex-col bg-slate-100 text-slate-900">
      <header className="flex items-center gap-6 border-b border-slate-200 bg-white px-6 py-3">
        <h1 className="text-lg font-semibold tracking-tight">Orquestrator</h1>
        <nav className="flex gap-1">
          {tabs.map((tab) => (
            <a
              key={tab.page}
              href={pageHref(tab.page)}
              aria-current={current === tab.page ? 'page' : undefined}
              className={`rounded-md px-3 py-1.5 text-sm font-medium ${
                current === tab.page
                  ? 'bg-slate-900 text-white'
                  : 'text-slate-600 hover:bg-slate-100'
              }`}
            >
              {tab.label}
            </a>
          ))}
        </nav>
      </header>
      <main className="min-h-0 flex-1 overflow-auto p-6">
        {(page === 'board' || (page === 'cards' && !id)) && <Board />}
        {page === 'cards' && id && <CardPage key={id} cardId={id} />}
        {page === 'projects' && <ProjectsPage />}
        {page === 'card-types' && <CardTypesPage />}
        {page === 'agents' && <AgentsPage />}
        {page === 'flows' && !id && <FlowsPage />}
        {page === 'flows' && id && <FlowEditor key={id} flowId={id} />}
      </main>
    </div>
  )
}

export default App
