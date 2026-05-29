<template>
  <section class="view active">
    <div class="list-toolbar">
      <div>
        <h2>{{ tApp('nav.my') }}</h2>
        <p class="meta">{{ tApp('my.menu.chooseHint') }}</p>
      </div>
    </div>

    <div class="my-layout">
      <aside class="my-sidebar" data-testid="my-menu-card">
        <nav class="my-nav">
          <button
            v-for="item in menuItems"
            :key="item.key"
            class="my-nav__item"
            :class="{ 'my-nav__item--active': activeMenu === item.key }"
            :data-testid="`my-menu-${item.key}`"
            type="button"
            @click="item.action ? item.action() : onMenuClick(item.key)"
          >
            <span class="my-nav__icon" v-html="item.icon" />
            <span class="my-nav__label">{{ item.label }}</span>
            <svg class="my-nav__arrow" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 18 15 12 9 6"/></svg>
          </button>
        </nav>
      </aside>

      <div class="my-content">
        <!-- Account Panel -->
        <div v-if="activeMenu === 'account'" class="my-panel" data-testid="my-account-panel">
          <div class="my-panel__header">
            <div class="my-panel__icon-wrap">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>
            </div>
            <div>
              <h3 class="my-panel__title">{{ tApp('my.account.title') }}</h3>
              <p class="my-panel__desc">{{ tApp('my.account.desc') }}</p>
            </div>
          </div>

          <div class="my-info-grid">
            <div class="my-info-row">
              <span class="my-info-row__label">{{ tApp('my.account.emailLabel') }}</span>
              <span class="my-info-row__value">{{ authStore.currentUser?.email || '-' }}</span>
            </div>
            <div class="my-info-row" data-testid="my-account-plan-row">
              <span class="my-info-row__label">{{ tApp('my.account.planLabel') }}</span>
              <span class="my-info-row__value my-info-row__value--plan">{{ currentPlanLabel }}</span>
            </div>
            <div class="my-info-row" data-testid="my-account-status-row">
              <span class="my-info-row__label">{{ tApp('my.account.statusLabel') }}</span>
              <span class="my-info-row__value">{{ currentStatusLabel }}</span>
            </div>
            <div
              v-if="planExpiryLabel"
              class="my-info-row"
              data-testid="my-account-plan-expiry-row"
            >
              <span class="my-info-row__label">{{ planExpiryRowLabel }}</span>
              <span class="my-info-row__value">{{ planExpiryLabel }}</span>
            </div>
            <div class="my-info-row">
              <span class="my-info-row__label">{{ tApp('my.account.deviceLimitLabel') }}</span>
              <span class="my-info-row__value">{{ currentDeviceLimit > 0 ? tApp('my.account.deviceLimitValue', { limit: currentDeviceLimit }) : '-' }}</span>
            </div>
            <div
              v-if="planExpiredBanner"
              class="my-info-row my-info-row--banner"
              data-testid="my-account-plan-expired-banner"
            >
              <span class="my-info-row__value my-info-row__value--banner">{{ planExpiredBanner }}</span>
            </div>
            <div class="my-info-row">
              <span class="my-info-row__label">{{ tApp('my.account.deviceLabel') }}</span>
              <span class="my-info-row__value my-info-row__value--mono">{{ authStore.state.deviceId || '-' }}</span>
            </div>
            <div class="my-info-row" data-testid="my-account-version-row">
              <span class="my-info-row__label">{{ tApp('my.account.versionLabel') }}</span>
              <span class="my-info-row__value my-info-row__value--mono">
                {{ appVersionLabel }}
                <span
                  v-if="updaterStore.hasUpdate"
                  class="my-update-pill"
                  data-testid="my-account-update-pill"
                >
                  {{ tApp('my.account.update.availableBadge') }}
                </span>
                <span
                  v-else-if="updaterStatusLabel"
                  class="my-update-status"
                  :data-testid="updaterStatusTestId"
                >
                  {{ updaterStatusLabel }}
                </span>
              </span>
            </div>
            <div
              v-if="updaterStore.hasUpdate"
              class="my-info-row my-info-row--update"
              data-testid="my-account-update-row"
            >
              <span class="my-info-row__label">{{ tApp('my.account.update.latestLabel') }}</span>
              <span class="my-info-row__value my-info-row__value--mono">
                {{ updaterStore.result.latest }}
                <a
                  v-if="updaterStore.result.releaseNotesUrl"
                  class="my-update-link"
                  :href="updaterStore.result.releaseNotesUrl"
                  target="_blank"
                  rel="noopener noreferrer"
                  data-testid="my-account-release-notes"
                >
                  {{ tApp('my.account.update.releaseNotes') }}
                </a>
              </span>
            </div>
          </div>

          <div class="my-actions">
            <button
              v-if="!authStore.isAuthenticated"
              class="btn primary"
              type="button"
              :disabled="authStore.loginBusy"
              data-testid="my-account-login"
              @click="onStartLogin"
            >
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/><polyline points="15 3 21 3 21 9"/><line x1="10" y1="14" x2="21" y2="3"/></svg>
              {{ authStore.loginBusy ? tApp('auth.login.starting') : tApp('auth.login.start') }}
            </button>
            <button
              class="btn primary"
              type="button"
              :disabled="!updaterStore.canOpenDownload || updateOpening"
              v-if="updaterStore.hasUpdate"
              data-testid="my-account-update-now"
              @click="onOpenUpdateDownload"
            >
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
              {{ tApp('my.account.update.updateNow') }}
            </button>
            <button
              class="btn secondary"
              type="button"
              :disabled="updaterStore.loading"
              data-testid="my-account-check-update"
              @click="onCheckForUpdate"
            >
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg>
              {{ updaterStore.loading ? tApp('my.account.update.checking') : tApp('my.account.update.checkButton') }}
            </button>
            <button v-if="authStore.isAuthenticated" class="btn secondary" data-testid="my-account-refresh-devices" type="button" @click="loadDevices">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg>
              {{ tApp('my.account.refreshDevices') }}
            </button>
            <button v-if="authStore.isAuthenticated" class="btn ghost danger" data-testid="my-account-logout" type="button" @click="onLogout">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><polyline points="16 17 21 12 16 7"/><line x1="21" y1="12" x2="9" y2="12"/></svg>
              {{ tApp('my.account.logout') }}
            </button>
          </div>

          <div class="my-devices">
            <div class="my-devices__header">
              <h4 class="my-devices__title">{{ tApp('my.account.devicesTitle') }}</h4>
              <span class="my-devices__count" v-if="authStore.devices.length">{{ authStore.devices.length }}</span>
            </div>
            <p v-if="currentDeviceLimit > 0" class="meta">{{ tApp('my.account.deviceUsage', { used: authStore.devices.length, limit: currentDeviceLimit }) }}</p>
            <div v-if="authStore.devicesLoading" class="my-devices__empty">
              <svg class="my-devices__spinner" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 12a9 9 0 1 1-6.219-8.56"/></svg>
              {{ tApp('my.account.loadingDevices') }}
            </div>
            <div v-else-if="!authStore.devices.length" class="my-devices__empty">
              {{ tApp('my.account.noDevices') }}
            </div>
            <div v-else class="my-devices__list">
              <div
                v-for="device in sortedDevices"
                :key="device.deviceId"
                class="my-device-card"
                :class="{ 'my-device-card--current': isCurrentDevice(device) }"
                :data-testid="isCurrentDevice(device) ? 'my-device-card-current' : 'my-device-card'"
              >
                <div class="my-device-card__icon">
                  <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="3" width="20" height="14" rx="2" ry="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/></svg>
                </div>
                <div class="my-device-card__info">
                  <div class="my-device-card__title-row">
                    <strong class="my-device-card__name">{{ deviceTitle(device) }}</strong>
                    <span v-if="isCurrentDevice(device)" class="my-device-card__badge">
                      <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>
                      {{ tApp('my.account.currentDevice') }}
                    </span>
                  </div>
                  <span class="my-device-card__meta">{{ formatPlatform(device.platform) || tApp('my.account.unknownPlatform') }}</span>
                  <span class="my-device-card__id">{{ device.deviceId }}</span>
                </div>
                <div class="my-device-card__actions">
                  <button
                    v-if="!isCurrentDevice(device)"
                    class="btn ghost danger mini"
                    type="button"
                    @click="onRemoveDevice(device.deviceId)"
                  >
                    {{ tApp('my.account.removeDevice') }}
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Language Panel -->
        <div v-else-if="activeMenu === 'language'" class="my-panel" data-testid="my-language-panel">
          <div class="my-panel__header">
            <div class="my-panel__icon-wrap">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/></svg>
            </div>
            <div>
              <h3 class="my-panel__title">{{ tApp('my.language.title') }}</h3>
              <p class="my-panel__desc">{{ tApp('my.language.desc') }}</p>
            </div>
          </div>

          <div class="my-lang-grid">
            <button
              v-for="loc in selectableLocales"
              :key="loc"
              class="my-lang-option"
              :class="{ 'my-lang-option--active': currentLocale === loc }"
              type="button"
              @click="selectLocale(loc)"
            >
              <span class="my-lang-option__flag">{{ localeFlags[loc] }}</span>
              <span class="my-lang-option__label">{{ tApp(`my.language.option.${loc}`) }}</span>
              <svg v-if="currentLocale === loc" class="my-lang-option__check" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>
            </button>
          </div>
        </div>

        <!-- Knowledge Base Panel -->
        <MyKnowledgeBaseView v-else-if="activeMenu === 'knowledge-base'" />

        <!-- AI Skill Panel -->
        <div v-else-if="activeMenu === 'skill'" class="my-panel" data-testid="my-skill-panel">
          <div class="my-panel__header">
            <div class="my-panel__icon-wrap">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M12 2L2 7l10 5 10-5-10-5z"/><path d="M2 17l10 5 10-5"/><path d="M2 12l10 5 10-5"/></svg>
            </div>
            <div>
              <h3 class="my-panel__title">{{ tApp('skill.manage.title') }}</h3>
              <p class="my-panel__desc">{{ tApp('skill.manage.desc') }}</p>
            </div>
          </div>

          <div class="my-agent-approval-policy" data-testid="my-agent-approval-policy">
            {{ tApp('skill.agentApprovalPolicyNotice') }}
          </div>

          <div class="my-skill-agents">
            <div
              v-for="agent in skillAgents"
              :key="agent.id"
              class="my-skill-card"
              :class="{ 'my-skill-card--disabled': !agent.detected }"
            >
              <div class="my-skill-card__main">
                <div class="my-skill-card__left">
                  <div class="my-skill-card__head">
                    <span class="my-skill-card__name">{{ agent.name }}</span>
                    <span v-if="agent.installed" class="my-skill-card__badge installed">{{ tApp('skill.install.alreadyInstalled') }}</span>
                    <span v-else-if="agent.detected" class="my-skill-card__badge ready">{{ tApp('skill.install.notInstalled') }}</span>
                    <span v-else class="my-skill-card__badge not-detected">{{ tApp('skill.install.notDetected') }}</span>
                    <span
                      v-if="identityForSkill(agent)?.revokedAt"
                      class="my-skill-card__badge revoked"
                      data-testid="my-skill-card-revoked-badge"
                    >
                      {{ tApp('skill.manage.revoked') }}
                    </span>
                  </div>
                  <span v-if="agent.detected" class="my-skill-card__path">{{ agent.installPath }}</span>
                </div>
                <button
                  v-if="agent.detected && agent.installed"
                  class="my-skill-card__btn uninstall"
                  type="button"
                  :disabled="skillBusy === agent.id"
                  @click="onUninstallAgent(agent)"
                >
                  {{ tApp('skill.manage.uninstall') }}
                </button>
                <button
                  v-else-if="agent.detected && !agent.installed"
                  class="my-skill-card__btn install"
                  type="button"
                  :disabled="skillBusy === agent.id"
                  @click="onInstallAgent(agent)"
                >
                  {{ tApp('skill.manage.install') }}
                </button>
              </div>
              <div
                v-if="identityForSkill(agent)"
                class="my-agent-identity"
                :data-testid="`my-agent-identity-skill-${agent.id}`"
              >
                <div class="my-agent-identity__row">
                  <label class="my-agent-identity__label" :for="`skill-name-${agent.id}`">
                    {{ tApp('skill.manage.agentNameLabel') }}
                  </label>
                  <input
                    :id="`skill-name-${agent.id}`"
                    class="my-agent-identity__input"
                    type="text"
                    :placeholder="tApp('skill.manage.agentNamePlaceholder')"
                    :disabled="renameBusyKey === identityForSkill(agent)!.accessKey"
                    :value="nameDrafts[identityForSkill(agent)!.accessKey] ?? identityForSkill(agent)!.name"
                    data-testid="my-agent-identity-name"
                    autocapitalize="off"
                    autocorrect="off"
                    spellcheck="false"
                    @input="onNameInput(identityForSkill(agent)!.accessKey, $event)"
                    @keydown.enter.prevent="onRenameIdentity(identityForSkill(agent)!)"
                  />
                  <button
                    class="my-skill-card__btn secondary"
                    type="button"
                    :disabled="renameBusyKey === identityForSkill(agent)!.accessKey || !isNameDirty(identityForSkill(agent)!)"
                    data-testid="my-agent-identity-save"
                    @click="onRenameIdentity(identityForSkill(agent)!)"
                  >
                    {{ renameBusyKey === identityForSkill(agent)!.accessKey ? tApp('skill.manage.renameSaving') : tApp('skill.manage.rename') }}
                  </button>
                </div>
                <div class="my-agent-identity__row">
                  <span class="my-agent-identity__label">{{ tApp('skill.manage.accessKeyLabel') }}</span>
                  <input
                    v-if="revealedKey === identityForSkill(agent)!.accessKey"
                    class="my-agent-identity__key-reveal"
                    type="text"
                    readonly
                    :aria-label="tApp('skill.manage.accessKeyLabel')"
                    :value="identityForSkill(agent)!.accessKey"
                    data-testid="my-agent-identity-key-revealed"
                    @focus="onRevealedKeyFocus"
                  />
                  <code v-else class="my-agent-identity__key" data-testid="my-agent-identity-key">{{ maskAccessKey(identityForSkill(agent)!.accessKey) }}</code>
                  <button
                    class="my-skill-card__btn secondary"
                    type="button"
                    data-testid="my-agent-identity-copy"
                    @click="onCopyAccessKey(identityForSkill(agent)!.accessKey)"
                  >
                    {{ copiedKey === identityForSkill(agent)!.accessKey ? tApp('skill.manage.keyCopied') : tApp('skill.manage.copyKey') }}
                  </button>
                  <button
                    v-if="revealedKey === identityForSkill(agent)!.accessKey"
                    class="my-skill-card__btn secondary"
                    type="button"
                    data-testid="my-agent-identity-hide"
                    @click="onHideRevealedKey"
                  >
                    {{ tApp('skill.manage.hideKey') }}
                  </button>
                  <button
                    v-if="!identityForSkill(agent)!.revokedAt"
                    class="my-skill-card__btn revoke"
                    type="button"
                    :disabled="revokeBusyKey === identityForSkill(agent)!.accessKey"
                    data-testid="my-agent-identity-revoke"
                    @click="openRevokeConfirm(identityForSkill(agent)!)"
                  >
                    {{ tApp('skill.manage.revoke') }}
                  </button>
                  <button
                    v-else
                    class="my-skill-card__btn install"
                    type="button"
                    :disabled="revokeBusyKey === identityForSkill(agent)!.accessKey"
                    data-testid="my-agent-identity-unrevoke"
                    @click="onUnrevokeIdentity(identityForSkill(agent)!)"
                  >
                    {{ tApp('skill.manage.unrevoke') }}
                  </button>
                </div>
                <div class="my-agent-identity__row my-agent-identity__row--grant">
                  <span class="my-agent-identity__label">{{ tApp('skill.manage.sensitivityGrantLabel') }}</span>
                  <span
                    class="my-agent-identity__grant-state"
                    :class="{ 'my-agent-identity__grant-state--on': identityForSkill(agent)!.sensitivityClassificationGrant }"
                    :data-testid="`my-agent-identity-grant-state-skill-${agent.id}`"
                  >
                    {{ identityForSkill(agent)!.sensitivityClassificationGrant ? tApp('skill.manage.sensitivityGrantOn') : tApp('skill.manage.sensitivityGrantOff') }}
                  </span>
                  <span class="my-agent-identity__grant-hint">{{ tApp('skill.manage.sensitivityGrantHint') }}</span>
                  <button
                    class="my-skill-card__btn"
                    :class="identityForSkill(agent)!.sensitivityClassificationGrant ? 'revoke' : 'install'"
                    type="button"
                    :disabled="grantBusyKey === identityForSkill(agent)!.accessKey"
                    :data-testid="`my-agent-identity-grant-toggle-skill-${agent.id}`"
                    @click="onToggleSensitivityGrant(identityForSkill(agent)!)"
                  >
                    {{ identityForSkill(agent)!.sensitivityClassificationGrant ? tApp('skill.manage.sensitivityGrantRevoke') : tApp('skill.manage.sensitivityGrantAllow') }}
                  </button>
                </div>
                <div class="my-agent-identity__row my-agent-identity__row--grant">
                  <span class="my-agent-identity__label">{{ tApp('skill.manage.datasourceGrantLabel') }}</span>
                  <span
                    class="my-agent-identity__grant-state"
                    :class="{ 'my-agent-identity__grant-state--on': identityForSkill(agent)!.datasourceManagementGrant }"
                    :data-testid="`my-agent-identity-datasource-grant-state-skill-${agent.id}`"
                  >
                    {{ identityForSkill(agent)!.datasourceManagementGrant ? tApp('skill.manage.datasourceGrantOn') : tApp('skill.manage.datasourceGrantOff') }}
                  </span>
                  <span class="my-agent-identity__grant-hint">{{ tApp('skill.manage.datasourceGrantHint') }}</span>
                  <button
                    class="my-skill-card__btn"
                    :class="identityForSkill(agent)!.datasourceManagementGrant ? 'revoke' : 'install'"
                    type="button"
                    :disabled="grantBusyKey === identityForSkill(agent)!.accessKey"
                    :data-testid="`my-agent-identity-datasource-grant-toggle-skill-${agent.id}`"
                    @click="onToggleDatasourceManagementGrant(identityForSkill(agent)!)"
                  >
                    {{ identityForSkill(agent)!.datasourceManagementGrant ? tApp('skill.manage.datasourceGrantRevoke') : tApp('skill.manage.datasourceGrantAllow') }}
                  </button>
                </div>
              </div>
            </div>
          </div>

          <!-- MCP Server Configuration -->
          <div class="my-panel__divider" />
          <div class="my-panel__header">
            <div class="my-panel__icon-wrap">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="2" width="20" height="8" rx="2" ry="2"/><rect x="2" y="14" width="20" height="8" rx="2" ry="2"/><line x1="6" y1="6" x2="6.01" y2="6"/><line x1="6" y1="18" x2="6.01" y2="18"/></svg>
            </div>
            <div>
              <h3 class="my-panel__title">{{ tApp('mcp.manage.title') }}</h3>
              <p class="my-panel__desc">{{ tApp('mcp.manage.desc') }}</p>
            </div>
          </div>

          <div class="my-skill-agents">
            <div
              v-for="agent in mcpAgents"
              :key="'mcp-' + agent.id"
              class="my-skill-card"
              :class="{ 'my-skill-card--disabled': !agent.detected }"
            >
              <div class="my-skill-card__main">
                <div class="my-skill-card__left">
                  <div class="my-skill-card__head">
                    <span class="my-skill-card__name">{{ agent.name }}</span>
                    <span v-if="agent.installed" class="my-skill-card__badge installed">{{ tApp('skill.install.alreadyInstalled') }}</span>
                    <span v-else-if="agent.detected" class="my-skill-card__badge ready">{{ tApp('skill.install.notInstalled') }}</span>
                    <span v-else class="my-skill-card__badge not-detected">{{ tApp('skill.install.notDetected') }}</span>
                    <span
                      v-if="identityForMCP(agent)?.revokedAt"
                      class="my-skill-card__badge revoked"
                      data-testid="my-mcp-card-revoked-badge"
                    >
                      {{ tApp('skill.manage.revoked') }}
                    </span>
                  </div>
                  <span v-if="agent.detected" class="my-skill-card__path">{{ agent.configPath }}</span>
                  <span
                    v-if="agent.id === 'codex'"
                    class="my-skill-card__hint"
                    data-testid="my-codex-mcp-hint"
                  >
                    {{ codexMCPHint(agent) }}
                  </span>
                </div>
                <button
                  v-if="agent.detected && agent.installed"
                  class="my-skill-card__btn uninstall"
                  type="button"
                  :disabled="mcpBusy === agent.id"
                  :data-testid="agent.id === 'codex' ? 'my-codex-disconnect-mcp' : undefined"
                  @click="onUninstallMCP(agent)"
                >
                  {{ agent.id === 'codex' ? tApp('mcp.manage.codexDisconnect') : tApp('skill.manage.uninstall') }}
                </button>
                <button
                  v-else-if="agent.detected && !agent.installed"
                  class="my-skill-card__btn install"
                  type="button"
                  :disabled="mcpBusy === agent.id"
                  :data-testid="agent.id === 'codex' ? 'my-codex-authorize-mcp' : undefined"
                  @click="onInstallMCP(agent)"
                >
                  {{ agent.id === 'codex' ? tApp('mcp.manage.codexAuthorize') : tApp('skill.manage.install') }}
                </button>
              </div>
              <div
                v-if="identityForMCP(agent)"
                class="my-agent-identity"
                :data-testid="`my-agent-identity-mcp-${agent.id}`"
              >
                <div class="my-agent-identity__row">
                  <label class="my-agent-identity__label" :for="`mcp-name-${agent.id}`">
                    {{ tApp('skill.manage.agentNameLabel') }}
                  </label>
                  <input
                    :id="`mcp-name-${agent.id}`"
                    class="my-agent-identity__input"
                    type="text"
                    :placeholder="tApp('skill.manage.agentNamePlaceholder')"
                    :disabled="renameBusyKey === identityForMCP(agent)!.accessKey"
                    :value="nameDrafts[identityForMCP(agent)!.accessKey] ?? identityForMCP(agent)!.name"
                    data-testid="my-agent-identity-name"
                    autocapitalize="off"
                    autocorrect="off"
                    spellcheck="false"
                    @input="onNameInput(identityForMCP(agent)!.accessKey, $event)"
                    @keydown.enter.prevent="onRenameIdentity(identityForMCP(agent)!)"
                  />
                  <button
                    class="my-skill-card__btn secondary"
                    type="button"
                    :disabled="renameBusyKey === identityForMCP(agent)!.accessKey || !isNameDirty(identityForMCP(agent)!)"
                    data-testid="my-agent-identity-save"
                    @click="onRenameIdentity(identityForMCP(agent)!)"
                  >
                    {{ renameBusyKey === identityForMCP(agent)!.accessKey ? tApp('skill.manage.renameSaving') : tApp('skill.manage.rename') }}
                  </button>
                </div>
                <div class="my-agent-identity__row">
                  <span class="my-agent-identity__label">{{ tApp('skill.manage.accessKeyLabel') }}</span>
                  <input
                    v-if="revealedKey === identityForMCP(agent)!.accessKey"
                    class="my-agent-identity__key-reveal"
                    type="text"
                    readonly
                    :aria-label="tApp('skill.manage.accessKeyLabel')"
                    :value="identityForMCP(agent)!.accessKey"
                    data-testid="my-agent-identity-key-revealed"
                    @focus="onRevealedKeyFocus"
                  />
                  <code v-else class="my-agent-identity__key" data-testid="my-agent-identity-key">{{ maskAccessKey(identityForMCP(agent)!.accessKey) }}</code>
                  <button
                    class="my-skill-card__btn secondary"
                    type="button"
                    data-testid="my-agent-identity-copy"
                    @click="onCopyAccessKey(identityForMCP(agent)!.accessKey)"
                  >
                    {{ copiedKey === identityForMCP(agent)!.accessKey ? tApp('skill.manage.keyCopied') : tApp('skill.manage.copyKey') }}
                  </button>
                  <button
                    v-if="revealedKey === identityForMCP(agent)!.accessKey"
                    class="my-skill-card__btn secondary"
                    type="button"
                    data-testid="my-agent-identity-hide"
                    @click="onHideRevealedKey"
                  >
                    {{ tApp('skill.manage.hideKey') }}
                  </button>
                  <button
                    v-if="!identityForMCP(agent)!.revokedAt"
                    class="my-skill-card__btn revoke"
                    type="button"
                    :disabled="revokeBusyKey === identityForMCP(agent)!.accessKey"
                    data-testid="my-agent-identity-revoke"
                    @click="openRevokeConfirm(identityForMCP(agent)!)"
                  >
                    {{ tApp('skill.manage.revoke') }}
                  </button>
                  <button
                    v-else
                    class="my-skill-card__btn install"
                    type="button"
                    :disabled="revokeBusyKey === identityForMCP(agent)!.accessKey"
                    data-testid="my-agent-identity-unrevoke"
                    @click="onUnrevokeIdentity(identityForMCP(agent)!)"
                  >
                    {{ tApp('skill.manage.unrevoke') }}
                  </button>
                </div>
                <div class="my-agent-identity__row my-agent-identity__row--grant">
                  <span class="my-agent-identity__label">{{ tApp('skill.manage.sensitivityGrantLabel') }}</span>
                  <span
                    class="my-agent-identity__grant-state"
                    :class="{ 'my-agent-identity__grant-state--on': identityForMCP(agent)!.sensitivityClassificationGrant }"
                    :data-testid="`my-agent-identity-grant-state-mcp-${agent.id}`"
                  >
                    {{ identityForMCP(agent)!.sensitivityClassificationGrant ? tApp('skill.manage.sensitivityGrantOn') : tApp('skill.manage.sensitivityGrantOff') }}
                  </span>
                  <span class="my-agent-identity__grant-hint">{{ tApp('skill.manage.sensitivityGrantHint') }}</span>
                  <button
                    class="my-skill-card__btn"
                    :class="identityForMCP(agent)!.sensitivityClassificationGrant ? 'revoke' : 'install'"
                    type="button"
                    :disabled="grantBusyKey === identityForMCP(agent)!.accessKey"
                    :data-testid="`my-agent-identity-grant-toggle-mcp-${agent.id}`"
                    @click="onToggleSensitivityGrant(identityForMCP(agent)!)"
                  >
                    {{ identityForMCP(agent)!.sensitivityClassificationGrant ? tApp('skill.manage.sensitivityGrantRevoke') : tApp('skill.manage.sensitivityGrantAllow') }}
                  </button>
                </div>
                <div class="my-agent-identity__row my-agent-identity__row--grant">
                  <span class="my-agent-identity__label">{{ tApp('skill.manage.datasourceGrantLabel') }}</span>
                  <span
                    class="my-agent-identity__grant-state"
                    :class="{ 'my-agent-identity__grant-state--on': identityForMCP(agent)!.datasourceManagementGrant }"
                    :data-testid="`my-agent-identity-datasource-grant-state-mcp-${agent.id}`"
                  >
                    {{ identityForMCP(agent)!.datasourceManagementGrant ? tApp('skill.manage.datasourceGrantOn') : tApp('skill.manage.datasourceGrantOff') }}
                  </span>
                  <span class="my-agent-identity__grant-hint">{{ tApp('skill.manage.datasourceGrantHint') }}</span>
                  <button
                    class="my-skill-card__btn"
                    :class="identityForMCP(agent)!.datasourceManagementGrant ? 'revoke' : 'install'"
                    type="button"
                    :disabled="grantBusyKey === identityForMCP(agent)!.accessKey"
                    :data-testid="`my-agent-identity-datasource-grant-toggle-mcp-${agent.id}`"
                    @click="onToggleDatasourceManagementGrant(identityForMCP(agent)!)"
                  >
                    {{ identityForMCP(agent)!.datasourceManagementGrant ? tApp('skill.manage.datasourceGrantRevoke') : tApp('skill.manage.datasourceGrantAllow') }}
                  </button>
                </div>
              </div>
            </div>
          </div>

          <!-- Manual-install Agents -->
          <div class="my-panel__divider" />
          <div class="my-panel__header">
            <div class="my-panel__icon-wrap">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="16 18 22 12 16 6"/><polyline points="8 6 2 12 8 18"/></svg>
            </div>
            <div>
              <h3 class="my-panel__title">{{ tApp('skill.manage.manualSectionTitle') }}</h3>
              <p class="my-panel__desc">{{ tApp('skill.manage.manualSectionDesc') }}</p>
            </div>
          </div>

          <div class="my-skill-agents" data-testid="my-manual-agents">
            <div class="my-skill-manual-banner">
              <span class="my-skill-manual-banner__text">{{ tApp('skill.manage.manualSectionDesc') }}</span>
              <button
                class="my-skill-manual-banner__btn"
                type="button"
                data-testid="my-manual-agent-new"
                :disabled="showNewManualAgent"
                @click="onCreateManualAgent"
              >
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
                {{ tApp('skill.manage.manualNewBtn') }}
              </button>
            </div>

            <div
              v-if="!manualIdentities.length"
              class="my-skill-card my-skill-card--disabled"
              data-testid="my-manual-agents-empty"
            >
              <div class="my-skill-card__main">
                <div class="my-skill-card__left">
                  <span class="my-skill-card__path">{{ tApp('skill.manage.manualEmpty') }}</span>
                </div>
              </div>
            </div>

            <div
              v-for="identity in manualIdentities"
              :key="`manual-${identity.accessKey}`"
              class="my-skill-card"
              :data-testid="`my-manual-agent-${identity.accessKey}`"
            >
              <div class="my-skill-card__main">
                <div class="my-skill-card__left">
                  <div class="my-skill-card__head">
                    <span class="my-skill-card__name">{{ identity.name || tApp('skill.manualInstall.agentNamePlaceholder') }}</span>
                    <span class="my-skill-card__badge installed">{{ tApp('skill.manage.manualBadge') }}</span>
                    <span
                      v-if="identity.revokedAt"
                      class="my-skill-card__badge revoked"
                    >
                      {{ tApp('skill.manage.revoked') }}
                    </span>
                  </div>
                  <span class="my-skill-card__path">
                    {{ tApp('skill.manage.manualPathLabel') }}: {{ tApp('skill.manage.manualPathValue') }}
                  </span>
                </div>
              </div>
              <div class="my-agent-identity">
                <div class="my-agent-identity__row">
                  <label class="my-agent-identity__label" :for="`manual-name-${identity.accessKey}`">
                    {{ tApp('skill.manage.agentNameLabel') }}
                  </label>
                  <input
                    :id="`manual-name-${identity.accessKey}`"
                    class="my-agent-identity__input"
                    type="text"
                    :placeholder="tApp('skill.manage.agentNamePlaceholder')"
                    :disabled="renameBusyKey === identity.accessKey"
                    :value="nameDrafts[identity.accessKey] ?? identity.name"
                    autocapitalize="off"
                    autocorrect="off"
                    spellcheck="false"
                    @input="onNameInput(identity.accessKey, $event)"
                    @keydown.enter.prevent="onRenameIdentity(identity)"
                  />
                  <button
                    class="my-skill-card__btn secondary"
                    type="button"
                    :disabled="renameBusyKey === identity.accessKey || !isNameDirty(identity)"
                    @click="onRenameIdentity(identity)"
                  >
                    {{ renameBusyKey === identity.accessKey ? tApp('skill.manage.renameSaving') : tApp('skill.manage.rename') }}
                  </button>
                </div>
                <div class="my-agent-identity__row">
                  <span class="my-agent-identity__label">{{ tApp('skill.manage.accessKeyLabel') }}</span>
                  <input
                    v-if="revealedKey === identity.accessKey"
                    class="my-agent-identity__key-reveal"
                    type="text"
                    readonly
                    :aria-label="tApp('skill.manage.accessKeyLabel')"
                    :value="identity.accessKey"
                    @focus="onRevealedKeyFocus"
                  />
                  <code v-else class="my-agent-identity__key">{{ maskAccessKey(identity.accessKey) }}</code>
                  <button
                    class="my-skill-card__btn secondary"
                    type="button"
                    @click="onCopyAccessKey(identity.accessKey)"
                  >
                    {{ copiedKey === identity.accessKey ? tApp('skill.manage.keyCopied') : tApp('skill.manage.copyKey') }}
                  </button>
                  <button
                    v-if="revealedKey === identity.accessKey"
                    class="my-skill-card__btn secondary"
                    type="button"
                    @click="onHideRevealedKey"
                  >
                    {{ tApp('skill.manage.hideKey') }}
                  </button>
                  <button
                    v-if="!identity.revokedAt"
                    class="my-skill-card__btn install"
                    type="button"
                    :data-testid="`my-manual-agent-show-install-${identity.accessKey}`"
                    @click="openManualInstallForKey(identity.accessKey)"
                  >
                    {{ tApp('skill.manage.showInstall') }}
                  </button>
                  <button
                    v-if="!identity.revokedAt"
                    class="my-skill-card__btn revoke"
                    type="button"
                    :disabled="revokeBusyKey === identity.accessKey"
                    @click="openRevokeConfirm(identity)"
                  >
                    {{ tApp('skill.manage.revoke') }}
                  </button>
                  <button
                    v-else
                    class="my-skill-card__btn install"
                    type="button"
                    :disabled="revokeBusyKey === identity.accessKey"
                    @click="onUnrevokeIdentity(identity)"
                  >
                    {{ tApp('skill.manage.unrevoke') }}
                  </button>
                </div>
                <div class="my-agent-identity__row my-agent-identity__row--grant">
                  <span class="my-agent-identity__label">{{ tApp('skill.manage.sensitivityGrantLabel') }}</span>
                  <span
                    class="my-agent-identity__grant-state"
                    :class="{ 'my-agent-identity__grant-state--on': identity.sensitivityClassificationGrant }"
                    :data-testid="`my-agent-identity-grant-state-manual-${identity.accessKey}`"
                  >
                    {{ identity.sensitivityClassificationGrant ? tApp('skill.manage.sensitivityGrantOn') : tApp('skill.manage.sensitivityGrantOff') }}
                  </span>
                  <span class="my-agent-identity__grant-hint">{{ tApp('skill.manage.sensitivityGrantHint') }}</span>
                  <button
                    class="my-skill-card__btn"
                    :class="identity.sensitivityClassificationGrant ? 'revoke' : 'install'"
                    type="button"
                    :disabled="grantBusyKey === identity.accessKey"
                    :data-testid="`my-agent-identity-grant-toggle-manual-${identity.accessKey}`"
                    @click="onToggleSensitivityGrant(identity)"
                  >
                    {{ identity.sensitivityClassificationGrant ? tApp('skill.manage.sensitivityGrantRevoke') : tApp('skill.manage.sensitivityGrantAllow') }}
                  </button>
                </div>
                <div class="my-agent-identity__row my-agent-identity__row--grant">
                  <span class="my-agent-identity__label">{{ tApp('skill.manage.datasourceGrantLabel') }}</span>
                  <span
                    class="my-agent-identity__grant-state"
                    :class="{ 'my-agent-identity__grant-state--on': identity.datasourceManagementGrant }"
                    :data-testid="`my-agent-identity-datasource-grant-state-manual-${identity.accessKey}`"
                  >
                    {{ identity.datasourceManagementGrant ? tApp('skill.manage.datasourceGrantOn') : tApp('skill.manage.datasourceGrantOff') }}
                  </span>
                  <span class="my-agent-identity__grant-hint">{{ tApp('skill.manage.datasourceGrantHint') }}</span>
                  <button
                    class="my-skill-card__btn"
                    :class="identity.datasourceManagementGrant ? 'revoke' : 'install'"
                    type="button"
                    :disabled="grantBusyKey === identity.accessKey"
                    :data-testid="`my-agent-identity-datasource-grant-toggle-manual-${identity.accessKey}`"
                    @click="onToggleDatasourceManagementGrant(identity)"
                  >
                    {{ identity.datasourceManagementGrant ? tApp('skill.manage.datasourceGrantRevoke') : tApp('skill.manage.datasourceGrantAllow') }}
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Settings Panel -->
        <div v-else-if="activeMenu === 'settings'" class="my-panel" data-testid="my-settings-panel">
          <AiChatPreferences />

          <div class="my-settings-divider" />

          <div class="my-settings-logs">
            <div class="my-panel__header">
              <div class="my-panel__icon-wrap">
                <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/><polyline points="10 9 9 9 8 9"/></svg>
              </div>
              <div>
                <h3 class="my-panel__title">{{ tApp('my.settings.title') }}</h3>
                <p class="my-panel__desc">{{ tApp('my.settings.desc') }}</p>
              </div>
            </div>

            <div class="my-info-grid">
              <div class="my-info-row">
                <span class="my-info-row__label">
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/></svg>
                  {{ tApp('my.settings.locationLabel') }}
                </span>
                <span class="my-info-row__value my-info-row__value--natural">{{ tApp('my.settings.locationValue') }}</span>
              </div>
              <div class="my-info-row">
                <span class="my-info-row__label">
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>
                  {{ tApp('my.settings.limitLabel') }}
                </span>
                <span class="my-info-row__value">{{ tApp('my.settings.limitValue') }}</span>
              </div>
            </div>

            <div class="my-settings-toggle" data-testid="my-settings-datasource-timing">
              <div class="my-settings-toggle__main">
                <span class="my-settings-toggle__icon">
                  <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M3 3v18h18"/><path d="M7 14l3-3 3 2 5-6"/><path d="M18 7h-4"/><path d="M18 7v4"/></svg>
                </span>
                <span class="my-settings-toggle__text">
                  <strong>{{ tApp('my.settings.datasourceTimingTitle') }}</strong>
                  <small>{{ tApp('my.settings.datasourceTimingDesc') }}</small>
                </span>
              </div>
              <button
                class="my-settings-switch"
                :class="{ 'my-settings-switch--on': datasourceTimingLogEnabled }"
                type="button"
                role="switch"
                :aria-checked="datasourceTimingLogEnabled"
                :aria-label="tApp('my.settings.datasourceTimingTitle')"
                :disabled="datasourceTimingBusy"
                data-testid="my-settings-datasource-timing-switch"
                @click="onToggleDatasourceTimingLog"
              >
                <span class="my-settings-switch__thumb" />
                <span class="my-settings-switch__label">
                  {{ datasourceTimingLogEnabled ? tApp('my.settings.datasourceTimingOn') : tApp('my.settings.datasourceTimingOff') }}
                </span>
              </button>
            </div>

            <button
              class="my-settings-export"
              data-testid="my-settings-export-logs"
              type="button"
              :disabled="exportingLogs"
              @click="onExportLogs"
            >
              <span class="my-settings-export__icon">
                <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
              </span>
              <span class="my-settings-export__text">
                <strong>{{ exportingLogs ? tApp('my.settings.exporting') : tApp('my.settings.exportLogs') }}</strong>
                <small>{{ tApp('my.settings.exportLogsDesc') }}</small>
              </span>
              <svg class="my-settings-export__arrow" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 18 15 12 9 6"/></svg>
            </button>
          </div>
        </div>

        <div v-else class="my-panel my-panel--empty">
          <div class="meta">{{ tApp('my.menu.chooseHint') }}</div>
        </div>
      </div>
    </div>

    <ManualInstallDialog
      v-if="showManualInstall"
      :access-key="manualInstallAccessKey"
      @close="closeManualInstall"
    />

    <NewManualAgentDialog
      v-if="showNewManualAgent"
      @created="onNewManualAgentCreated"
      @close="closeNewManualAgent"
    />

    <div
      v-if="revokeConfirmTarget"
      class="dialog-backdrop"
      role="dialog"
      aria-modal="true"
      data-testid="my-agent-revoke-confirm"
    >
      <div class="dialog-card">
        <div class="dialog-head">
          <div class="dialog-head-main">
            <div class="dialog-icon">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>
            </div>
            <div>
              <h4>{{ tApp('skill.manage.revokeConfirmTitle') }}</h4>
              <div class="meta">
                <span>{{ tApp('skill.manage.revokeConfirmBody') }}</span>
              </div>
            </div>
          </div>
        </div>
        <div class="dialog-actions">
          <button class="btn ghost" type="button" :disabled="revokeBusyKey === revokeConfirmTarget.accessKey" @click="closeRevokeConfirm">
            {{ tApp('common.cancel') }}
          </button>
          <button
            class="btn danger"
            type="button"
            :disabled="revokeBusyKey === revokeConfirmTarget.accessKey"
            data-testid="my-agent-revoke-confirm-btn"
            @click="onRevokeIdentity(revokeConfirmTarget)"
          >
            {{ tApp('skill.manage.revokeConfirm') }}
          </button>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { useUpdaterStore } from '@/stores/updater'
import { api } from '@/services/api'
import AiChatPreferences from '@/components/ai/AiChatPreferences.vue'
import MyKnowledgeBaseView from '@/views/MyKnowledgeBaseView.vue'
import ManualInstallDialog from '@/components/skill/ManualInstallDialog.vue'
import NewManualAgentDialog from '@/components/skill/NewManualAgentDialog.vue'
import type { AgentIdentity, SkillAgent, MCPAgent } from '@/services/api/skill'
import { useRoute, useRouter } from 'vue-router'
import { appLocaleRef, setAppLocale, tApp, type AppLocale } from '@/modules/i18n/appI18n'
import { deviceLimitForPlan, planLabel } from '@/modules/plan/limits'
import type { EffectiveStatus } from '@/modules/plan/limits'
import { GetAppVersion } from '@wailsjs/go/main/App'

const store = useAppStore()
const authStore = useAuthStore()
const updaterStore = useUpdaterStore()
const route = useRoute() as { query?: Record<string, unknown> } | undefined
const router = useRouter() as { replace?: (payload: { query: Record<string, string> }) => unknown; push?: (payload: any) => unknown } | undefined

type MenuKey = 'account' | 'language' | 'knowledge-base' | 'skill' | 'settings'
const validMenuKeys: MenuKey[] = ['account', 'language', 'knowledge-base', 'skill', 'settings']

const routeTab = typeof route?.query?.tab === 'string' ? route.query.tab : ''
const initMenu = (validMenuKeys as string[]).includes(routeTab) ? (routeTab as MenuKey) : 'account'
const activeMenu = ref<MenuKey>(initMenu)

function syncQueryToRoute() {
  router?.replace?.({ query: { tab: activeMenu.value } })
}
const currentLocale = computed(() => appLocaleRef.value)
const currentPlanLabel = computed(() => {
  if (authStore.effectiveLicense.effectiveStatus === 'trial') return tApp('plan.name.trial')
  return planLabel(authStore.effectivePlan) || '-'
})
const currentStatusLabel = computed(() => {
  const status: EffectiveStatus = authStore.effectiveLicense.effectiveStatus
  if (status === 'trial') return tApp('my.account.statusValue.trial')
  if (status === 'pro_expired') return tApp('my.account.statusValue.proExpired')
  if (status === 'active') return tApp('my.account.statusValue.active')
  if (authStore.currentLicense?.status) return authStore.currentLicense.status
  return '-'
})
const planExpiryDateLabel = computed(() => {
  const expiresAt = authStore.effectiveLicense.effectiveStatus === 'trial'
    ? authStore.effectiveLicense.trialExpiresAt
    : authStore.currentLicense?.expiresAt ?? 0
  if (!expiresAt) return ''
  // expiresAt is unix seconds from the backend
  const date = new Date(expiresAt * 1000)
  if (Number.isNaN(date.getTime())) return ''
  try {
    return date.toLocaleString(currentLocale.value === 'zh' ? 'zh-CN' : undefined)
  } catch {
    return date.toISOString()
  }
})
const planExpiryRowLabel = computed(() => {
  if (!planExpiryDateLabel.value) return ''
  if (authStore.effectiveLicense.effectiveStatus === 'trial') return tApp('my.account.trialExpiresLabel')
  // Hide the row for Free users — there is no Pro expiry to show.
  if (authStore.effectiveLicense.rawPlan !== 'pro') return ''
  const status = authStore.effectiveLicense.effectiveStatus
  return status === 'pro_expired'
    ? tApp('my.account.expiredOnLabel')
    : tApp('my.account.expiresLabel')
})
const planExpiryLabel = computed(() => (planExpiryRowLabel.value ? planExpiryDateLabel.value : ''))
const planExpiredBanner = computed(() => {
  if (authStore.effectiveLicense.effectiveStatus !== 'pro_expired') return ''
  return tApp('my.account.planExpiredBanner')
})
const currentDeviceLimit = computed(() => {
  if (authStore.effectiveLicense.effectiveStatus === 'trial') {
    return deviceLimitForPlan(authStore.effectivePlan) || 0
  }
  if (!authStore.currentLicense?.plan) return 0
  return authStore.deviceLimit || deviceLimitForPlan(authStore.effectivePlan) || 0
})

const formatPlatform = (raw: string | undefined | null): string => {
  const value = String(raw || '').trim().toLowerCase()
  if (!value) return ''
  if (value === 'macos' || value === 'darwin' || value === 'mac') return 'macOS'
  if (value === 'windows' || value === 'win32' || value === 'win') return 'Windows'
  if (value === 'linux') return 'Linux'
  if (value === 'ios') return 'iOS'
  if (value === 'android') return 'Android'
  return value.charAt(0).toUpperCase() + value.slice(1)
}

const isCurrentDevice = (device: { deviceId?: string }) =>
  Boolean(device.deviceId) && device.deviceId === authStore.state.deviceId

const deviceTitle = (device: { deviceName?: string; deviceId?: string; platform?: string }) => {
  const name = String(device.deviceName || '').trim()
  if (name) return name
  const platform = formatPlatform(device.platform)
  if (platform) return tApp('my.account.unnamedDeviceOn', { platform })
  return tApp('my.account.unnamedDevice')
}

const sortedDevices = computed(() => {
  const list = authStore.devices.slice()
  list.sort((a, b) => {
    const aCurrent = isCurrentDevice(a) ? 0 : 1
    const bCurrent = isCurrentDevice(b) ? 0 : 1
    if (aCurrent !== bCurrent) return aCurrent - bCurrent
    return (b.lastActiveAt || 0) - (a.lastActiveAt || 0)
  })
  return list
})
const selectableLocales: AppLocale[] = ['en', 'zh', 'ja', 'es', 'de']
const currentLanguageLabel = computed(() => tApp(`my.language.option.${currentLocale.value}`))
const exportingLogs = ref(false)
const datasourceTimingLogEnabled = ref(false)
const datasourceTimingBusy = ref(false)
const appVersion = ref('')
const appVersionLabel = computed(() => {
  const v = appVersion.value.trim()
  if (!v || v === 'dev') return tApp('my.account.versionFallback')
  return v
})
const updateOpening = ref(false)
const updaterStatusLabel = computed(() => {
  if (updaterStore.loading) return tApp('my.account.update.checking')
  if (updaterStore.error) return tApp('my.account.update.error', { message: updaterStore.error })
  if (!updaterStore.result.lastCheckedAt) return ''
  if (!updaterStore.result.authenticated) return tApp('my.account.update.signInHint')
  if (!updaterStore.result.latest) return ''
  return tApp('my.account.update.upToDate')
})
const updaterStatusTestId = computed(() => {
  if (updaterStore.loading) return 'my-account-update-checking'
  if (updaterStore.error) return 'my-account-update-error'
  if (!updaterStore.result.authenticated && updaterStore.result.lastCheckedAt) return 'my-account-update-signin'
  return 'my-account-update-uptodate'
})
const skillAgents = ref<SkillAgent[]>([])
const skillBusy = ref<string | null>(null)
const mcpAgents = ref<MCPAgent[]>([])
const mcpBusy = ref<string | null>(null)
const showManualInstall = ref(false)
// Seed the manual install dialog with a specific identity's access key so the
// snippets render with the correct agent_* token. The "view snippets" button
// on each manual-agent card always passes a key; the legacy empty-key entry
// (the top "手动安装" banner) was removed in TASK-20260425-101011.
const manualInstallAccessKey = ref('')
const showNewManualAgent = ref(false)

const agentIdentities = ref<AgentIdentity[]>([])
const nameDrafts = ref<Record<string, string>>({})
const renameBusyKey = ref('')
const revokeBusyKey = ref('')
const copiedKey = ref('')

const manualIdentities = computed<AgentIdentity[]>(() =>
  agentIdentities.value
    .filter((identity) => identity.source === 'manual')
    .slice()
    .sort((a, b) => {
      const left = a.createdAt || ''
      const right = b.createdAt || ''
      return left < right ? -1 : left > right ? 1 : 0
    }),
)
// When a clipboard copy fails (secure-context denial, permission prompt rejected,
// execCommand missing in a locked-down WebView), show the raw key in a readonly
// input so the user can still select and copy it manually. Without this the UI
// only exposes a masked value and manual recovery is impossible.
const revealedKey = ref('')
const revokeConfirmTarget = ref<AgentIdentity | null>(null)
let copyResetTimer: ReturnType<typeof setTimeout> | null = null

const openManualInstallForKey = (accessKey: string) => {
  manualInstallAccessKey.value = accessKey
  showManualInstall.value = true
}

const closeManualInstall = () => {
  showManualInstall.value = false
  manualInstallAccessKey.value = ''
  // The dialog can mutate identity state in two ways:
  //   - GetManualInstallInfo() lazily mints the default manual identity on first open
  //   - the in-dialog name input persists renames via RenameAgentIdentity on blur/Enter
  // Reload so the manual-agents section reflects those changes immediately
  // instead of staying stale until the next tab switch.
  void loadAgentIdentities()
}

const onCreateManualAgent = () => {
  // Stage 1 of NewManualAgentDialog handles name input + CreateManualAgent
  // call without window.prompt — required because WKWebView (Wails on macOS)
  // disables window.prompt and previously made this button look unresponsive.
  showNewManualAgent.value = true
}

const onNewManualAgentCreated = (identity: AgentIdentity) => {
  // Stage 1 succeeded; identity is persisted server-side. Notify the user
  // immediately so the toast appears even if they close the dialog before
  // reading the snippet stage.
  store.setNotice(tApp('skill.manage.manualNewSuccess', { name: identity.name }), 'success')
}

const closeNewManualAgent = async () => {
  showNewManualAgent.value = false
  // Refresh so the freshly minted card shows up in the manual-agents list.
  // No-op if the user cancelled before submitting.
  await loadAgentIdentities()
}

// Primary lookup is by access key: the skill file / MCP config stores the key
// verbatim, and the backend returns it on each agent. Path-keyed lookup is
// kept as a fallback for identities stored before AccessKey was exposed, but
// is intentionally best-effort — backend normalizeInstallPath lowercases and
// resolves symlinks, which the browser cannot reproduce.
const identityByKey = computed(() => {
  const byKey = new Map<string, AgentIdentity>()
  agentIdentities.value.forEach((identity) => {
    if (identity.accessKey) {
      byKey.set(identity.accessKey, identity)
    }
  })
  return byKey
})

const identityByPath = computed(() => {
  const byKey = new Map<string, AgentIdentity>()
  agentIdentities.value.forEach((identity) => {
    if (!identity.installPath || !identity.agentType) return
    byKey.set(`${identity.agentType}::${identity.installPath}`, identity)
  })
  return byKey
})

const identityForSkill = (agent: SkillAgent): AgentIdentity | undefined => {
  if (agent.accessKey) {
    const match = identityByKey.value.get(agent.accessKey)
    if (match) return match
  }
  if (!agent.installPath) return undefined
  return identityByPath.value.get(`${agent.id}::${agent.installPath}`)
}

const identityForMCP = (agent: MCPAgent): AgentIdentity | undefined => {
  if (agent.accessKey) {
    const match = identityByKey.value.get(agent.accessKey)
    if (match) return match
  }
  if (!agent.configPath) return undefined
  return identityByPath.value.get(`${agent.id}::${agent.configPath}`)
}

const codexMCPHint = (agent: MCPAgent) => {
  if (agent.installed) return tApp('mcp.manage.codexAuthorizedHint')
  if (agent.detected) return tApp('mcp.manage.codexDetectedHint')
  return tApp('mcp.manage.codexNotDetectedHint')
}

const maskAccessKey = (accessKey: string) => {
  const value = String(accessKey || '')
  if (value.length === 0) return ''
  // Mirror backend MaskAccessKey in internal/agentaudit/runtime.go: short
  // keys have too little entropy to mask safely, so we collapse to *** so
  // a stray log or screenshot never leaks the whole thing.
  if (value.length <= 10) return '***'
  return `${value.slice(0, 6)}…${value.slice(-4)}`
}

const isNameDirty = (identity: AgentIdentity) => {
  const draft = nameDrafts.value[identity.accessKey]
  if (draft === undefined) return false
  const next = draft.trim()
  return next.length > 0 && next !== (identity.name || '').trim()
}

const onNameInput = (accessKey: string, event: Event) => {
  const target = event.target as HTMLInputElement | null
  nameDrafts.value = { ...nameDrafts.value, [accessKey]: target?.value ?? '' }
}

const loadAgentIdentities = async () => {
  try {
    agentIdentities.value = await api.listAgentIdentities()
  } catch {
    // non-critical
  }
}

const onRenameIdentity = async (identity: AgentIdentity) => {
  const draft = (nameDrafts.value[identity.accessKey] ?? identity.name ?? '').trim()
  if (!draft || draft === (identity.name || '').trim()) return
  renameBusyKey.value = identity.accessKey
  try {
    await api.renameAgentIdentity(identity.accessKey, draft)
    store.setNotice(tApp('skill.manage.renameSuccess', { name: draft }), 'success')
    nameDrafts.value = { ...nameDrafts.value, [identity.accessKey]: draft }
    await loadAgentIdentities()
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err || '')
    store.setNotice(tApp('skill.manage.renameError', { message }), 'error')
  } finally {
    renameBusyKey.value = ''
  }
}

const openRevokeConfirm = (identity: AgentIdentity) => {
  revokeConfirmTarget.value = identity
}

const closeRevokeConfirm = () => {
  if (revokeBusyKey.value === revokeConfirmTarget.value?.accessKey) return
  revokeConfirmTarget.value = null
}

const onRevokeIdentity = async (identity: AgentIdentity) => {
  revokeBusyKey.value = identity.accessKey
  try {
    await api.revokeAgentIdentity(identity.accessKey)
    store.setNotice(tApp('skill.manage.revokeSuccess'), 'success')
    await loadAgentIdentities()
    revokeConfirmTarget.value = null
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err || '')
    store.setNotice(tApp('skill.manage.revokeError', { message }), 'error')
  } finally {
    revokeBusyKey.value = ''
  }
}

const onUnrevokeIdentity = async (identity: AgentIdentity) => {
  revokeBusyKey.value = identity.accessKey
  try {
    await api.unrevokeAgentIdentity(identity.accessKey)
    store.setNotice(tApp('skill.manage.unrevokeSuccess'), 'success')
    await loadAgentIdentities()
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err || '')
    store.setNotice(tApp('skill.manage.unrevokeError', { message }), 'error')
  } finally {
    revokeBusyKey.value = ''
  }
}

const grantBusyKey = ref('')

const onToggleSensitivityGrant = async (identity: AgentIdentity) => {
  if (grantBusyKey.value === identity.accessKey) return
  grantBusyKey.value = identity.accessKey
  const next = !identity.sensitivityClassificationGrant
  try {
    await api.setAgentSensitivityGrant(identity.accessKey, next)
    store.setNotice(
      tApp(next ? 'skill.manage.sensitivityGrantOnSuccess' : 'skill.manage.sensitivityGrantOffSuccess', { name: identity.name || identity.accessKey }),
      'success',
    )
    await loadAgentIdentities()
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err || '')
    store.setNotice(tApp('skill.manage.sensitivityGrantError', { message }), 'error')
  } finally {
    grantBusyKey.value = ''
  }
}

const onToggleDatasourceManagementGrant = async (identity: AgentIdentity) => {
  if (grantBusyKey.value === identity.accessKey) return
  grantBusyKey.value = identity.accessKey
  const next = !identity.datasourceManagementGrant
  try {
    await api.setAgentDatasourceManagementGrant(identity.accessKey, next)
    store.setNotice(
      tApp(next ? 'skill.manage.datasourceGrantOnSuccess' : 'skill.manage.datasourceGrantOffSuccess', { name: identity.name || identity.accessKey }),
      'success',
    )
    await loadAgentIdentities()
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err || '')
    store.setNotice(tApp('skill.manage.datasourceGrantError', { message }), 'error')
  } finally {
    grantBusyKey.value = ''
  }
}

const copyToClipboard = async (text: string): Promise<boolean> => {
  if (!text) return false
  if (navigator?.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text)
      return true
    } catch {
      // Secure-context or permission failure — fall through to textarea fallback.
    }
  }
  // Fallback: if execCommand or anything else throws after the textarea is
  // appended, the node (and the secret inside it) must still be removed.
  // A try/finally guarantees cleanup; without it the access key would
  // linger in the DOM, visible to anyone who opens devtools.
  let el: HTMLTextAreaElement | null = null
  try {
    el = document.createElement('textarea')
    el.value = text
    el.setAttribute('readonly', '')
    el.style.position = 'fixed'
    el.style.opacity = '0'
    document.body.appendChild(el)
    el.select()
    return typeof document.execCommand === 'function' && !!document.execCommand('copy')
  } catch {
    return false
  } finally {
    if (el && el.parentNode) {
      el.value = ''
      el.parentNode.removeChild(el)
    }
  }
}

const onCopyAccessKey = async (accessKey: string) => {
  const ok = await copyToClipboard(accessKey)
  if (!ok) {
    revealedKey.value = accessKey
    store.setNotice(tApp('skill.manage.copyFailed'), 'error')
    return
  }
  revealedKey.value = ''
  copiedKey.value = accessKey
  if (copyResetTimer) clearTimeout(copyResetTimer)
  copyResetTimer = setTimeout(() => {
    copiedKey.value = ''
  }, 1800)
}

const onHideRevealedKey = () => {
  revealedKey.value = ''
}

const onRevealedKeyFocus = (event: FocusEvent) => {
  const target = event.target as HTMLInputElement | null
  if (target) target.select()
}

const localeFlags: Record<string, string> = {
  en: '🇺🇸',
  zh: '🇨🇳',
  ja: '🇯🇵',
  es: '🇪🇸',
  de: '🇩🇪',
}

const menuItems = computed(() => [
  {
    key: 'account' as const,
    label: tApp('my.menu.account'),
    icon: '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>',
    action: openAccountPanel,
  },
  {
    key: 'language' as const,
    label: tApp('my.menu.language'),
    icon: '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/></svg>',
  },
  {
    key: 'knowledge-base' as const,
    label: tApp('my.menu.knowledgeBase'),
    icon: '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20"/><path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z"/></svg>',
  },
  {
    key: 'skill' as const,
    label: tApp('my.menu.skill'),
    icon: '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M12 2L2 7l10 5 10-5-10-5z"/><path d="M2 17l10 5 10-5"/><path d="M2 12l10 5 10-5"/></svg>',
    action: openSkillPanel,
  },
  {
    key: 'settings' as const,
    label: tApp('my.menu.settings'),
    icon: '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>',
  },
])

const selectLocale = (locale: AppLocale) => {
  setAppLocale(locale)
  store.setNotice(tApp('my.language.saved'), 'success')
}

const onLocaleChange = (event: Event) => {
  const rawValue = String((event.target as HTMLSelectElement | null)?.value || '')
  const value = (selectableLocales.find((locale) => locale === rawValue) || 'en') as AppLocale
  selectLocale(value)
}

function onMenuClick(key: MenuKey) {
  activeMenu.value = key
  syncQueryToRoute()
}

const onExportLogs = async () => {
  if (exportingLogs.value) return
  exportingLogs.value = true
  try {
    const path = await api.exportLogs()
    store.setNotice(tApp('my.settings.exportSuccess', { path }), 'success')
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err || '')
    store.setNotice(tApp('my.settings.exportError', { message }), 'error')
  } finally {
    exportingLogs.value = false
  }
}

const loadDiagnosticsSettings = async () => {
  try {
    const settings = await api.getDiagnosticsSettings()
    datasourceTimingLogEnabled.value = Boolean(settings?.datasourceTimingLogEnabled)
  } catch {
    // Non-critical: the export action still works when backend diagnostics
    // settings are unavailable in a test or mock runtime.
  }
}

const onToggleDatasourceTimingLog = async () => {
  if (datasourceTimingBusy.value) return
  const next = !datasourceTimingLogEnabled.value
  datasourceTimingBusy.value = true
  try {
    const settings = await api.setDatasourceTimingLogEnabled(next)
    datasourceTimingLogEnabled.value = Boolean(settings?.datasourceTimingLogEnabled)
    store.setNotice(tApp(datasourceTimingLogEnabled.value ? 'my.settings.datasourceTimingEnabled' : 'my.settings.datasourceTimingDisabled'), 'success')
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err || '')
    store.setNotice(tApp('my.settings.datasourceTimingError', { message }), 'error')
  } finally {
    datasourceTimingBusy.value = false
  }
}

const loadDevices = async () => {
  try {
    await authStore.loadDevices()
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err || '')
    store.setNotice(tApp('my.account.loadDevicesError', { message }), 'error')
  }
}

const openAccountPanel = async () => {
  activeMenu.value = 'account'
  syncQueryToRoute()
  await loadDevices()
}

const onLogout = async () => {
  try {
    await authStore.logout()
    store.setNotice(tApp('my.account.logoutSuccess'), 'success')
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err || '')
    store.setNotice(tApp('my.account.logoutError', { message }), 'error')
  }
}

const onStartLogin = async () => {
  try {
    await authStore.startLogin()
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err || '')
    store.setNotice(message, 'error')
  }
}

const onRemoveDevice = async (deviceId: string) => {
  try {
    await authStore.removeDevice(deviceId)
    store.setNotice(tApp('my.account.removeSuccess'), 'success')
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err || '')
    store.setNotice(tApp('my.account.removeError', { message }), 'error')
  }
}

const loadSkillAgents = async () => {
  try {
    skillAgents.value = await api.detectAIAgents()
  } catch {
    // non-critical
  }
}

const loadMCPAgents = async () => {
  try {
    mcpAgents.value = await api.detectMCPAgents()
  } catch {
    // non-critical
  }
}

const openSkillPanel = async () => {
  activeMenu.value = 'skill'
  syncQueryToRoute()
  await Promise.all([loadSkillAgents(), loadMCPAgents(), loadAgentIdentities()])
}

const onInstallAgent = async (agent: SkillAgent) => {
  skillBusy.value = agent.id
  try {
    const result = await api.installSkill([agent.id])
    const outcome = result.installed?.[0]
    if (outcome?.success) {
      store.setNotice(tApp('skill.manage.installSuccess', { name: agent.name }), 'success')
    } else {
      store.setNotice(tApp('skill.manage.installError', { name: agent.name, message: outcome?.error || '' }), 'error')
    }
    await Promise.all([loadSkillAgents(), loadAgentIdentities()])
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err || '')
    store.setNotice(tApp('skill.manage.installError', { name: agent.name, message }), 'error')
  } finally {
    skillBusy.value = null
  }
}

const onUninstallAgent = async (agent: SkillAgent) => {
  skillBusy.value = agent.id
  try {
    const result = await api.uninstallSkill([agent.id])
    const outcome = result.installed?.[0]
    if (outcome?.success) {
      store.setNotice(tApp('skill.manage.uninstallSuccess', { name: agent.name }), 'success')
    } else {
      store.setNotice(tApp('skill.manage.uninstallError', { name: agent.name, message: outcome?.error || '' }), 'error')
    }
    await Promise.all([loadSkillAgents(), loadAgentIdentities()])
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err || '')
    store.setNotice(tApp('skill.manage.uninstallError', { name: agent.name, message }), 'error')
  } finally {
    skillBusy.value = null
  }
}

const onInstallMCP = async (agent: MCPAgent) => {
  mcpBusy.value = agent.id
  try {
    const result = await api.installMCP([agent.id])
    const outcome = result.installed?.[0]
    if (outcome?.success) {
      store.setNotice(tApp('mcp.manage.installSuccess', { name: agent.name }), 'success')
    } else {
      store.setNotice(tApp('mcp.manage.installError', { name: agent.name, message: outcome?.error || '' }), 'error')
    }
    await Promise.all([loadMCPAgents(), loadAgentIdentities()])
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err || '')
    store.setNotice(tApp('mcp.manage.installError', { name: agent.name, message }), 'error')
  } finally {
    mcpBusy.value = null
  }
}

const onUninstallMCP = async (agent: MCPAgent) => {
  mcpBusy.value = agent.id
  try {
    const result = await api.uninstallMCP([agent.id])
    const outcome = result.installed?.[0]
    if (outcome?.success) {
      store.setNotice(tApp('mcp.manage.uninstallSuccess', { name: agent.name }), 'success')
    } else {
      store.setNotice(tApp('mcp.manage.uninstallError', { name: agent.name, message: outcome?.error || '' }), 'error')
    }
    await Promise.all([loadMCPAgents(), loadAgentIdentities()])
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err || '')
    store.setNotice(tApp('mcp.manage.uninstallError', { name: agent.name, message }), 'error')
  } finally {
    mcpBusy.value = null
  }
}


onMounted(() => {
  if (activeMenu.value === 'account') {
    void loadDevices()
  } else if (activeMenu.value === 'skill') {
    void Promise.all([loadSkillAgents(), loadMCPAgents(), loadAgentIdentities()])
  }
  void loadDiagnosticsSettings()
  void loadAppVersion()
})

const onCheckForUpdate = async () => {
  await updaterStore.check()
  if (updaterStore.error) {
    store.setNotice(tApp('my.account.update.error', { message: updaterStore.error }), 'error')
    return
  }
  if (!updaterStore.result.authenticated && updaterStore.result.lastCheckedAt) {
    store.setNotice(tApp('my.account.update.signInHint'), 'info')
    return
  }
  if (updaterStore.hasUpdate) {
    store.setNotice(tApp('my.account.update.availableNotice', { latest: updaterStore.result.latest }), 'success')
  } else if (updaterStore.result.lastCheckedAt) {
    store.setNotice(tApp('my.account.update.upToDate'), 'success')
  }
}

const onOpenUpdateDownload = async () => {
  if (updateOpening.value) return
  updateOpening.value = true
  try {
    await updaterStore.openDownload()
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err || '')
    store.setNotice(tApp('my.account.update.openError', { message }), 'error')
  } finally {
    updateOpening.value = false
  }
}

async function loadAppVersion() {
  try {
    const w = (window as unknown as { go?: { main?: { App?: { GetAppVersion?: unknown } } } }).go
    if (!w?.main?.App?.GetAppVersion) return
    appVersion.value = (await GetAppVersion()) || ''
  } catch {
    appVersion.value = ''
  }
}
</script>

<style scoped>
/* ── Layout ── */
.my-layout {
  display: grid;
  gap: 16px;
  grid-template-columns: 240px minmax(0, 1fr);
}

@media (max-width: 768px) {
  .my-layout {
    grid-template-columns: 1fr;
  }
}

/* ── Sidebar Nav ── */
.my-sidebar {
  position: sticky;
  top: 12px;
  align-self: start;
}

@media (max-width: 768px) {
  .my-sidebar {
    position: static;
  }
}

.my-nav {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.my-nav__item {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  padding: 10px 12px;
  border: none;
  border-radius: 10px;
  background: transparent;
  color: var(--soft-ink);
  font: inherit;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.2s ease;
  text-align: left;
  position: relative;
}

.my-nav__item:hover {
  background: var(--surface);
  color: var(--ink);
}

.my-nav__item--active {
  background: color-mix(in oklab, var(--primary) 12%, transparent);
  color: var(--primary);
  font-weight: 700;
}

.my-nav__item--active .my-nav__icon {
  color: var(--primary);
}

.my-nav__icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: 8px;
  background: var(--surface);
  color: var(--soft-ink);
  flex-shrink: 0;
  transition: all 0.2s ease;
}

.my-nav__item--active .my-nav__icon {
  background: color-mix(in oklab, var(--primary) 18%, transparent);
  color: var(--primary);
}

.my-nav__item:hover .my-nav__icon {
  color: var(--ink);
}

.my-nav__label {
  flex: 1;
}

.my-nav__arrow {
  opacity: 0;
  transform: translateX(-4px);
  transition: all 0.2s ease;
  flex-shrink: 0;
}

.my-nav__item:hover .my-nav__arrow,
.my-nav__item--active .my-nav__arrow {
  opacity: 0.5;
  transform: translateX(0);
}

.my-nav__item--active .my-nav__arrow {
  opacity: 0.8;
  color: var(--primary);
}

/* ── Panel ── */
.my-panel {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.my-panel__header {
  display: flex;
  align-items: flex-start;
  gap: 14px;
}

.my-panel__icon-wrap {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  border-radius: 12px;
  background: color-mix(in oklab, var(--primary) 10%, var(--surface));
  border: 1px solid color-mix(in oklab, var(--primary) 15%, var(--edge));
  color: var(--primary);
  flex-shrink: 0;
}

.my-panel__title {
  margin: 0;
  font-size: 16px;
  font-weight: 700;
  color: var(--ink);
}

.my-panel__desc {
  margin: 4px 0 0;
  font-size: 12px;
  color: var(--soft-ink);
}

/* ── Info Grid ── */
.my-info-grid {
  display: flex;
  flex-direction: column;
  gap: 1px;
  border-radius: 12px;
  overflow: hidden;
  border: 1px solid var(--edge);
  background: var(--edge);
}

.my-info-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 14px;
  background: var(--panel-strong);
  font-size: 13px;
}

.my-info-row__label {
  color: var(--soft-ink);
  font-weight: 500;
  flex-shrink: 0;
}

.my-info-row__value {
  color: var(--ink);
  font-weight: 600;
  text-align: right;
  word-break: break-all;
}

.my-info-row__value--plan {
  color: var(--primary);
  text-transform: capitalize;
}

.my-info-row__value--mono {
  font-family: "SF Mono", "Fira Code", "Cascadia Code", monospace;
  font-size: 11px;
  font-weight: 500;
  opacity: 0.8;
}

.my-info-row__value--natural {
  max-width: 62%;
  line-height: 1.4;
  word-break: normal;
  overflow-wrap: normal;
}

.my-info-row--update {
  background: color-mix(in oklab, var(--primary) 6%, var(--panel-strong));
}

.my-update-pill {
  display: inline-flex;
  align-items: center;
  margin-left: 8px;
  padding: 1px 8px;
  border-radius: 999px;
  background: color-mix(in oklab, var(--primary) 18%, transparent);
  color: var(--primary);
  font-family: "Inter", system-ui, sans-serif;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.2px;
  text-transform: uppercase;
}

.my-update-status {
  display: inline-flex;
  margin-left: 8px;
  font-family: "Inter", system-ui, sans-serif;
  font-size: 11px;
  font-weight: 500;
  color: var(--soft-ink);
  opacity: 0.85;
}

.my-update-link {
  margin-left: 8px;
  font-family: "Inter", system-ui, sans-serif;
  font-size: 11px;
  color: var(--primary);
  text-decoration: none;
}

.my-update-link:hover {
  text-decoration: underline;
}

/* ── Actions ── */
.my-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.my-actions .btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

/* ── Devices ── */
.my-devices {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.my-devices__header {
  display: flex;
  align-items: center;
  gap: 8px;
}

.my-devices__title {
  margin: 0;
  font-size: 14px;
  font-weight: 600;
  color: var(--ink);
}

.my-devices__count {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 20px;
  height: 20px;
  padding: 0 6px;
  border-radius: 10px;
  background: color-mix(in oklab, var(--primary) 12%, transparent);
  color: var(--primary);
  font-size: 11px;
  font-weight: 700;
}

.my-devices__empty {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 16px;
  border-radius: 10px;
  background: var(--surface);
  border: 1px dashed var(--edge);
  font-size: 12px;
  color: var(--soft-ink);
}

.my-devices__spinner {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.my-devices__list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.my-device-card {
  position: relative;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 14px;
  border-radius: 12px;
  border: 1px solid var(--edge);
  background: var(--panel);
  transition: all 0.2s ease;
}

.my-device-card:hover {
  border-color: color-mix(in oklab, var(--primary) 20%, var(--edge));
  box-shadow: 0 2px 8px color-mix(in oklab, var(--primary) 6%, transparent);
}

.my-device-card--current {
  border-color: color-mix(in oklab, var(--primary) 55%, var(--edge));
  background: color-mix(in oklab, var(--primary) 7%, var(--panel));
  box-shadow: 0 0 0 1px color-mix(in oklab, var(--primary) 35%, transparent);
}

.my-device-card--current::before {
  content: "";
  position: absolute;
  top: 10px;
  bottom: 10px;
  left: 0;
  width: 3px;
  border-radius: 0 3px 3px 0;
  background: var(--primary);
}

.my-device-card__title-row {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.my-device-card__icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border-radius: 10px;
  background: var(--surface);
  color: var(--soft-ink);
  flex-shrink: 0;
}

.my-device-card--current .my-device-card__icon {
  background: color-mix(in oklab, var(--primary) 12%, transparent);
  color: var(--primary);
}

.my-device-card__info {
  display: flex;
  flex-direction: column;
  gap: 2px;
  flex: 1;
  min-width: 0;
}

.my-device-card__name {
  font-size: 13px;
  font-weight: 600;
  color: var(--ink);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.my-device-card__meta {
  font-size: 11px;
  color: var(--soft-ink);
}

.my-device-card__id {
  font-size: 10px;
  font-family: "SF Mono", "Fira Code", monospace;
  color: var(--soft-ink);
  opacity: 0.7;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.my-device-card__actions {
  flex-shrink: 0;
}

.my-device-card__badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px 10px;
  border-radius: 999px;
  background: color-mix(in oklab, var(--success) 12%, transparent);
  color: var(--success);
  font-size: 11px;
  font-weight: 600;
  white-space: nowrap;
}

/* ── Language Grid ── */
.my-lang-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  gap: 8px;
}

.my-lang-option {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 14px;
  border: 1px solid var(--edge);
  border-radius: 12px;
  background: var(--panel);
  font: inherit;
  font-size: 13px;
  font-weight: 500;
  color: var(--ink);
  cursor: pointer;
  transition: all 0.2s ease;
  text-align: left;
}

.my-lang-option:hover {
  border-color: color-mix(in oklab, var(--primary) 25%, var(--edge));
  background: color-mix(in oklab, var(--primary) 4%, var(--panel));
}

.my-lang-option--active {
  border-color: color-mix(in oklab, var(--primary) 35%, var(--edge));
  background: color-mix(in oklab, var(--primary) 8%, var(--panel));
}

.my-lang-option__flag {
  font-size: 20px;
  line-height: 1;
}

.my-lang-option__label {
  flex: 1;
}

.my-lang-option__check {
  color: var(--primary);
  flex-shrink: 0;
}

/* ── Info Row with icon ── */
.my-info-row__label {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.my-info-row__label svg {
  opacity: 0.5;
  flex-shrink: 0;
}

/* ── Section Dividers ── */
.my-panel__divider {
  height: 1px;
  background: var(--edge);
  margin: 20px 0 8px;
}

.my-settings-divider {
  height: 1px;
  background: var(--edge);
}

/* ── Settings Logs Section ── */
.my-settings-logs {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.my-settings-toggle {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  width: 100%;
  padding: 14px 16px;
  border-radius: 12px;
  border: 1px solid var(--edge);
  background: var(--panel);
}

.my-settings-toggle__main {
  display: flex;
  align-items: center;
  gap: 14px;
  min-width: 0;
}

.my-settings-toggle__icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  border-radius: 10px;
  background: color-mix(in oklab, var(--primary) 10%, var(--surface));
  color: var(--primary);
  flex: 0 0 auto;
}

.my-settings-toggle__text {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.my-settings-toggle__text strong {
  font-size: 13px;
  font-weight: 600;
  color: var(--ink);
}

.my-settings-toggle__text small {
  font-size: 11px;
  line-height: 1.4;
  color: var(--soft-ink);
}

.my-settings-switch {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-width: 88px;
  min-height: 32px;
  padding: 4px 10px 4px 4px;
  border-radius: 999px;
  border: 1px solid var(--edge);
  background: color-mix(in oklab, var(--soft-ink) 8%, var(--surface));
  color: var(--soft-ink);
  font: inherit;
  cursor: pointer;
  flex: 0 0 auto;
  white-space: nowrap;
}

.my-settings-switch--on {
  border-color: color-mix(in oklab, var(--primary) 45%, var(--edge));
  background: color-mix(in oklab, var(--primary) 14%, var(--surface));
  color: var(--primary);
}

.my-settings-switch:disabled {
  opacity: 0.65;
  cursor: default;
}

.my-settings-switch__thumb {
  width: 22px;
  height: 22px;
  border-radius: 999px;
  background: currentColor;
  opacity: 0.9;
  flex: 0 0 auto;
}

.my-settings-switch__label {
  font-size: 12px;
  font-weight: 600;
}

/* ── Export Button ── */
.my-settings-export {
  display: flex;
  align-items: center;
  gap: 14px;
  width: 100%;
  padding: 14px 16px;
  border-radius: 12px;
  border: 1px solid var(--edge);
  background: var(--panel);
  cursor: pointer;
  font: inherit;
  text-align: left;
  transition: all 0.2s ease;
}

.my-settings-export:hover {
  border-color: color-mix(in oklab, var(--primary) 30%, var(--edge));
  background: color-mix(in oklab, var(--primary) 4%, var(--panel));
  box-shadow: 0 4px 12px color-mix(in oklab, var(--primary) 8%, transparent);
}

.my-settings-export:active {
  transform: translateY(1px);
}

.my-settings-export:disabled {
  opacity: 0.6;
  cursor: default;
  transform: none;
}

.my-settings-export__icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  border-radius: 10px;
  background: color-mix(in oklab, var(--primary) 10%, var(--surface));
  color: var(--primary);
  flex-shrink: 0;
}

.my-settings-export__text {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.my-settings-export__text strong {
  font-size: 13px;
  font-weight: 600;
  color: var(--ink);
}

.my-settings-export__text small {
  font-size: 11px;
  color: var(--soft-ink);
}

.my-settings-export__arrow {
  color: var(--soft-ink);
  opacity: 0.4;
  flex-shrink: 0;
  transition: all 0.2s ease;
}

.my-settings-export:hover .my-settings-export__arrow {
  opacity: 0.8;
  color: var(--primary);
  transform: translateX(2px);
}

/* ── Skill Panel ── */
.my-skill-agents {
  display: grid;
  gap: 8px;
}

.my-agent-approval-policy {
  padding: 10px 14px;
  border-radius: 8px;
  border: 1px solid color-mix(in oklab, var(--warn, #f59e0b) 28%, var(--edge));
  background: color-mix(in oklab, var(--warn, #f59e0b) 9%, var(--panel));
  color: var(--ink);
  font-size: 12px;
  line-height: 1.5;
}

.my-skill-manual-banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 14px;
  border-radius: 10px;
  background: color-mix(in oklab, var(--primary) 6%, var(--panel));
  border: 1px dashed color-mix(in oklab, var(--primary) 28%, var(--edge));
  flex-wrap: wrap;
}

.my-skill-manual-banner__text {
  font-size: 12px;
  color: var(--soft-ink);
  flex: 1;
  min-width: 200px;
  line-height: 1.5;
}

.my-skill-manual-banner__btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  flex: 0 0 auto;
  min-height: 32px;
  padding: 5px 12px;
  border-radius: 8px;
  border: 1px solid color-mix(in oklab, var(--primary) 35%, var(--edge));
  background: var(--panel);
  color: var(--primary);
  font: inherit;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  transition: background 0.15s, border-color 0.15s;
  white-space: nowrap;
}

.my-skill-manual-banner__btn:hover {
  background: color-mix(in oklab, var(--primary) 10%, var(--panel));
}

.my-skill-card {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 12px 16px;
  border-radius: 10px;
  border: 1px solid color-mix(in oklab, var(--ink) 8%, transparent);
  background: var(--panel);
  transition: border-color 0.15s, box-shadow 0.15s;
}

.my-skill-card__main {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  min-width: 0;
}

.my-skill-card:hover:not(.my-skill-card--disabled) {
  border-color: color-mix(in oklab, var(--primary) 24%, transparent);
  box-shadow: 0 1px 4px color-mix(in oklab, var(--primary) 6%, transparent);
}

.my-skill-card--disabled {
  opacity: 0.45;
}

.my-skill-card__left {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.my-skill-card__head {
  display: flex;
  align-items: center;
  gap: 8px;
}

.my-skill-card__name {
  font-size: 13px;
  font-weight: 600;
  color: var(--ink);
}

.my-skill-card__badge {
  font-size: 11px;
  font-weight: 500;
  padding: 1px 8px;
  border-radius: 9999px;
  white-space: nowrap;
  line-height: 1.6;
}

.my-skill-card__badge.installed {
  background: color-mix(in oklab, var(--success, #22c55e) 12%, var(--panel));
  color: var(--success, #16a34a);
}

.my-skill-card__badge.ready {
  background: color-mix(in oklab, var(--primary) 10%, var(--panel));
  color: var(--primary);
}

.my-skill-card__badge.not-detected {
  background: color-mix(in oklab, var(--soft-ink) 8%, var(--panel));
  color: var(--soft-ink);
}

.my-skill-card__path {
  font-size: 11px;
  color: var(--soft-ink);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.my-skill-card__hint {
  font-size: 12px;
  color: var(--soft-ink);
  line-height: 1.45;
  max-width: 620px;
}

.my-skill-card__btn {
  flex: 0 0 auto;
  font-size: 12px;
  font-weight: 500;
  min-height: 32px;
  padding: 5px 14px;
  border-radius: 8px;
  border: 1px solid transparent;
  cursor: pointer;
  transition: background 0.15s, border-color 0.15s, opacity 0.15s;
  white-space: nowrap;
}

.my-skill-card__btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.my-skill-card__btn.install {
  background: var(--primary);
  color: #fff;
}

.my-skill-card__btn.install:hover:not(:disabled) {
  opacity: 0.88;
}

.my-skill-card__btn.uninstall {
  background: transparent;
  color: var(--soft-ink);
  border-color: color-mix(in oklab, var(--ink) 12%, transparent);
}

.my-skill-card__btn.uninstall:hover:not(:disabled) {
  color: var(--destructive, #ef4444);
  border-color: color-mix(in oklab, var(--destructive, #ef4444) 30%, transparent);
  background: color-mix(in oklab, var(--destructive, #ef4444) 5%, transparent);
}

.my-skill-card__btn.secondary {
  background: var(--panel-strong, var(--panel));
  color: var(--soft-ink);
  border-color: color-mix(in oklab, var(--ink) 10%, transparent);
}

.my-skill-card__btn.secondary:hover:not(:disabled) {
  color: var(--ink);
  border-color: color-mix(in oklab, var(--primary) 26%, transparent);
}

.my-skill-card__btn.revoke {
  background: transparent;
  color: color-mix(in oklab, var(--destructive, #ef4444) 80%, var(--ink));
  border-color: color-mix(in oklab, var(--destructive, #ef4444) 30%, transparent);
}

.my-skill-card__btn.revoke:hover:not(:disabled) {
  background: color-mix(in oklab, var(--destructive, #ef4444) 8%, transparent);
  border-color: color-mix(in oklab, var(--destructive, #ef4444) 45%, transparent);
}

.my-skill-card__badge.revoked {
  background: color-mix(in oklab, var(--destructive, #ef4444) 12%, var(--panel));
  color: color-mix(in oklab, var(--destructive, #ef4444) 85%, var(--ink));
}

.my-agent-identity {
  display: grid;
  gap: 8px;
  padding: 10px 12px;
  border-radius: 8px;
  border: 1px dashed color-mix(in oklab, var(--ink) 10%, var(--edge));
  background: color-mix(in oklab, var(--primary) 3%, transparent);
}

.my-agent-identity__row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.my-agent-identity__label {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--soft-ink);
  min-width: 84px;
}

.my-agent-identity__input {
  flex: 1 1 180px;
  min-height: 30px;
  padding: 0 10px;
  border: 1px solid var(--edge);
  border-radius: var(--control-radius, 8px);
  background: var(--panel);
  color: var(--ink);
  font: inherit;
  font-size: 12px;
}

.my-agent-identity__input:disabled {
  opacity: 0.5;
}

.my-agent-identity__key {
  flex: 1 1 180px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  color: var(--ink);
  padding: 4px 8px;
  border-radius: 6px;
  background: var(--panel-strong, var(--panel));
  border: 1px solid var(--edge);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}

.my-agent-identity__key-reveal {
  flex: 1 1 180px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  color: var(--ink);
  padding: 4px 8px;
  border-radius: 6px;
  background: var(--panel-strong, var(--panel));
  border: 1px solid var(--warn, var(--edge));
  min-width: 0;
  outline: none;
}

.my-agent-identity__row--grant {
  border-top: 1px dashed color-mix(in oklab, var(--ink) 10%, var(--edge));
  padding-top: 8px;
}

.my-agent-identity__grant-state {
  font-size: 12px;
  font-weight: 600;
  padding: 2px 8px;
  border-radius: 999px;
  background: color-mix(in oklab, var(--ink) 8%, var(--panel));
  color: var(--soft-ink);
  border: 1px solid var(--edge);
}

.my-agent-identity__grant-state--on {
  background: color-mix(in oklab, var(--primary) 14%, var(--panel));
  color: var(--primary);
  border-color: color-mix(in oklab, var(--primary) 30%, var(--edge));
}

.my-agent-identity__grant-hint {
  font-size: 11px;
  color: var(--soft-ink);
  flex: 1 1 180px;
  line-height: 1.5;
}

</style>
