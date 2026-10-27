import type { AgentProfile } from '../api/client'

/** The agent CLIs a profile can run. */
export const agentKindLabels: Record<AgentProfile['kind'], string> = {
  claude: 'Claude Code',
  opencode: 'opencode',
}
