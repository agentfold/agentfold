import { useBoardEvents } from './api/events'
import { Board } from './board/Board'
import { CardTypesPage } from './cardTypes/CardTypesPage'
import { ProjectsPage } from './projects/ProjectsPage'
import { pageHref, usePage, type Page } from './route'

const tabs: { page: Page; label: string }[] = [
  { page: 'board', label: 'Board' },
  { page: 'projects', label: 'Projetos' },
  { page: 'card-types', label: 'Tipos de card' },
]

function App() {
  const page = usePage()
  useBoardEvents()

  return (
    <div className="flex h-screen flex-col bg-slate-100 text-slate-900">
      <header className="flex items-center gap-6 border-b border-slate-200 bg-white px-6 py-3">
        <h1 className="text-lg font-semibold tracking-tight">Orquestrator</h1>
        <nav className="flex gap-1">
          {tabs.map((tab) => (
            <a
              key={tab.page}
              href={pageHref(tab.page)}
              aria-current={page === tab.page ? 'page' : undefined}
              className={`rounded-md px-3 py-1.5 text-sm font-medium ${
                page === tab.page
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
        {page === 'board' && <Board />}
        {page === 'projects' && <ProjectsPage />}
        {page === 'card-types' && <CardTypesPage />}
      </main>
    </div>
  )
}

export default App
