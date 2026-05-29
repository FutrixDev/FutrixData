import { createRouter, createWebHashHistory } from 'vue-router'
import DatasourceListView from '@/views/DatasourceListView.vue'
import DatasourceFormView from '@/views/DatasourceFormView.vue'
import ConsoleView from '@/views/ConsoleView.vue'
import HistoryView from '@/views/HistoryView.vue'
import SensitivityListView from '@/views/SensitivityListView.vue'
import RiskRulesView from '@/views/RiskRulesView.vue'
import RiskRulesFormView from '@/views/RiskRulesFormView.vue'
import AISettingsView from '@/views/AISettingsView.vue'
import AISettingsFormView from '@/views/AISettingsFormView.vue'
import MyView from '@/views/MyView.vue'
import SensitivityView from '@/views/SensitivityView.vue'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    {
      path: '/',
      name: 'datasources',
      component: DatasourceListView,
      meta: { titleKey: 'route.datasources' },
    },
    {
      path: '/datasources/new',
      name: 'datasource-create',
      component: DatasourceFormView,
      meta: { titleKey: 'route.datasourceCreate' },
    },
    {
      path: '/datasources/:id/edit',
      name: 'datasource-edit',
      component: DatasourceFormView,
      meta: { titleKey: 'route.datasourceEdit' },
    },
    {
      path: '/console',
      redirect: '/',
    },
    {
      path: '/console/:id',
      name: 'console',
      component: ConsoleView,
      meta: { titleKey: 'route.console' },
    },
    {
      path: '/history',
      name: 'history',
      component: HistoryView,
      meta: { titleKey: 'route.history' },
    },
    {
      path: '/sensitivity',
      name: 'sensitivity-list',
      component: SensitivityListView,
      meta: { titleKey: 'route.sensitivityList' },
    },
    {
      path: '/risk-rules',
      name: 'risk-rules',
      component: RiskRulesView,
      meta: { titleKey: 'route.riskRules' },
    },
    {
      path: '/risk-rules/new',
      name: 'risk-rules-create',
      component: RiskRulesFormView,
      meta: { titleKey: 'route.riskRulesCreate' },
    },
    {
      path: '/risk-rules/:id/edit',
      name: 'risk-rules-edit',
      component: RiskRulesFormView,
      meta: { titleKey: 'route.riskRulesEdit' },
    },
    {
      path: '/ai-settings',
      name: 'ai-settings',
      component: AISettingsView,
      meta: { titleKey: 'route.aiSettings' },
    },
    {
      path: '/ai-settings/new',
      name: 'ai-settings-create',
      component: AISettingsFormView,
      meta: { titleKey: 'route.aiSettingsCreate' },
    },
    {
      path: '/ai-settings/:id/edit',
      name: 'ai-settings-edit',
      component: AISettingsFormView,
      meta: { titleKey: 'route.aiSettingsEdit' },
    },
    {
      path: '/ai-settings/embedding/new',
      name: 'ai-settings-embedding-create',
      component: AISettingsFormView,
      meta: { titleKey: 'route.aiSettingsCreate' },
    },
    {
      path: '/ai-settings/embedding/:id/edit',
      name: 'ai-settings-embedding-edit',
      component: AISettingsFormView,
      meta: { titleKey: 'route.aiSettingsEdit' },
    },
    {
      path: '/sensitivity/:id',
      name: 'sensitivity-detail',
      component: SensitivityView,
      meta: { titleKey: 'route.sensitivity' },
    },
    {
      path: '/my',
      name: 'my',
      component: MyView,
      meta: { titleKey: 'route.my' },
    },
  ],
  scrollBehavior() {
    return { left: 0, top: 0 }
  },
})

export default router
