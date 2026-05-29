import fs from 'node:fs'
import path from 'node:path'

import { describe, expect, it } from 'vitest'

const projectRoot = path.resolve(__dirname, '..')

const readSource = (relativePath: string) =>
  fs.readFileSync(path.join(projectRoot, relativePath), 'utf8')

describe('static wording i18n coverage', () => {
  it('replaces hardcoded wording in core views/components with i18n keys', () => {
    const checks: Array<{ file: string; forbidden: string[] }> = [
      {
        file: 'views/DatasourceListView.vue',
        forbidden: [
          'Data Sources',
          'Manage connections and jump into the console.',
          'Test All',
          'New Data Source',
          'Open Console',
          'Delete datasource',
        ],
      },
      {
        file: 'views/datasource-list/useDatasourceListView.ts',
        forbidden: [
          "'Connected'",
          "'Failed'",
          "'Testing'",
          "'Unknown'",
          "'Copied.'",
          "'Datasource deleted.'",
          "'No endpoint to copy.'",
        ],
      },
      {
        file: 'views/DatasourceFormView.vue',
        forbidden: [
          'Configure connection details and test connectivity.',
          '>Cancel<',
          '>Save<',
          'Quick install (Docker)',
          'Test Connection',
        ],
      },
      {
        file: 'views/datasource-form/useDatasourceFormView.ts',
        forbidden: [
          "'Name is required.'",
          "'Type is required.'",
          "'Host is required.'",
          "'Port is required.'",
          "'Options must be valid JSON.'",
          "'Connected'",
          "'Failed'",
        ],
      },
      {
        file: 'views/HistoryView.vue',
        forbidden: [
          '<h2>History</h2>',
          'Review executed statements across datasources.',
          'Clear Filtered',
          'Clear Filters',
          'Loading history...',
          'No history yet.',
        ],
      },
      {
        file: 'views/history/useHistoryView.ts',
        forbidden: [
          "label: 'Datasource'",
          "label: 'Target'",
          "label: 'Database'",
          "'Targets'",
          "'Target'",
          "'Cleared {removed} entries.'",
        ],
      },
      {
        file: 'views/VisualizationView.vue',
        forbidden: [
          '<h2>Visualization</h2>',
          'Render AI-generated visualizations with Vega-Lite, ECharts, and Three.js.',
          'Clear History',
          '<h3>History</h3>',
          'Unsupported renderer',
        ],
      },
      {
        file: 'components/AIConfigPanel.vue',
        forbidden: [
          '<h3>AI Settings</h3>',
          'Manage providers for copilots and assistants.',
          'Add Provider',
          'Needs attention',
          'Delete AI provider',
        ],
      },
      {
        file: 'components/useAIConfigPanel.ts',
        forbidden: [
          "'Connected'",
          "'Failed'",
          "'Testing...'",
          "'AI provider deleted'",
        ],
      },
      {
        file: 'components/AIConfigForm.vue',
        forbidden: [
          'Set credentials, endpoints, and models.',
          'Configuration Name',
          '>Provider<',
          'Test Connection',
          'API key is required.',
        ],
      },
      {
        file: 'views/console/components/ConsoleToolbar.vue',
        forbidden: [
          '<h2 id="console-title">Console</h2>',
          '>Back<',
          'Switch Datasource',
          'Refresh Entities',
        ],
      },
      {
        file: 'views/console/composables/useConsoleViewLabels.ts',
        forbidden: [
          "'Select a datasource to begin.'",
          "'No entities found.'",
          "'Filter applies locally.'",
        ],
      },
      {
        file: 'views/console/components/ConsoleEntitiesPanel.vue',
        forbidden: [
          '>Refresh<',
          '>Databases<',
          'Refresh Entities',
          'Loading details...',
          'No details available.',
        ],
      },
      {
        file: 'views/console/components/ConsoleStatementPanel.vue',
        forbidden: [
          'aria-label="Statement tabs"',
          'New statement tab',
          'Type a statement to execute',
          '>Execute<',
          '>Execute All<',
          '>Beautify<',
          'Current target',
          'Analyze (Postgres)',
        ],
      },
      {
        file: 'views/console/components/ConsoleResultsContent.vue',
        forbidden: [
          '<h2>Result 1</h2>',
          '>All fields<',
          '>Export<',
          'Filter results...',
          '>Page size<',
          'No results yet.',
          '>Copy JSON<',
        ],
      },
      {
        file: 'views/console/components/ConsoleDangerDialogs.vue',
        forbidden: [
          'Confirm Redis command',
          'This command may block Redis or affect availability.',
          'High risk',
          'Run anyway',
        ],
      },
      {
        file: 'views/console/components/ConsoleResultsPanel.vue',
        forbidden: [
          '<h4>Results</h4>',
          'Expanded view',
          '>Close<',
        ],
      },
      {
        file: 'views/console/components/RedisKeyInspector.vue',
        forbidden: [
          'Key Inspector',
          'New Key',
          'Copy Key',
          'Clear output',
          'No preview items.',
          'Command Output',
        ],
      },
      {
        file: 'views/console/components/ConsoleVisualizationBuilder.vue',
        forbidden: [
          'Choose chart settings, then open in Visualization.',
          'No simple fields available for visualization.',
          'Open Visualization',
        ],
      },
      {
        file: 'components/VirtualTable.vue',
        forbidden: [
          '>Copy<',
          'aria-label="Copy row"',
          '0 rows.',
        ],
      },
      {
        file: 'components/VirtualMongoList.vue',
        forbidden: [
          'more fields',
          'Copy document',
          'Document structure',
          'No document details.',
          'Raw JSON',
          '0 documents.',
        ],
      },
      {
        file: 'components/ThemeToggle.vue',
        forbidden: [
          'Switch to',
        ],
      },
      {
        file: 'components/ai/AiSidebar.vue',
        forbidden: [
          'AI Chat',
          'New chat',
          'Approval required',
          'Reject',
          'Approve',
          'Voice input',
        ],
      },
      {
        file: 'components/ai/AiQuickPrompt.vue',
        forbidden: [
          'Remove context',
          'Ask AI...',
          'aria-label="Send"',
        ],
      },
      {
        file: 'components/ai/AiChatPreferences.vue',
        forbidden: [
          'AI Chat Preferences',
          'Default open',
          'Conversation retention',
        ],
      },
      {
        file: 'views/ConsoleView.vue',
        forbidden: [
          'Resize entities and editor panels',
          'Resize editor and results panels',
        ],
      },
      {
        file: 'components/ConsoleMonacoEditor.vue',
        forbidden: [
          "label: 'Format statement'",
        ],
      },
      {
        file: 'views/console/composables/useConsoleView.ts',
        forbidden: [
          "'Use db.<collection>.<method>(...) for beautify.'",
          "'Add arguments before beautify.'",
          "'Invalid Mongo statement. Fix syntax before beautify.'",
          "'Failed to beautify SQL.'",
        ],
      },
      {
        file: 'views/console/composables/useConsoleResults.ts',
        forbidden: [
          "'Row copied.'",
          "'Page copied.'",
          "'Mongo results copied.'",
        ],
      },
      {
        file: 'views/console/composables/useConsoleLifecycle.ts',
        forbidden: [
          "'No datasources available.'",
        ],
      },
      {
        file: 'views/console/composables/useConsoleHistory.ts',
        forbidden: [
          "'History entry does not match current datasource.'",
        ],
      },
    ]

    checks.forEach(({ file, forbidden }) => {
      const source = readSource(file)
      forbidden.forEach((literal) => {
        expect(source, `${file} still contains: ${literal}`).not.toContain(literal)
      })
    })
  })
})
