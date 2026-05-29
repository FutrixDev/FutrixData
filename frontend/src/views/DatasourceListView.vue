<template>
  <section class="view active" id="view-list">
    <div class="list-toolbar">
      <div>
        <h2>{{ tApp('datasource.list.title') }}</h2>
        <p class="meta">{{ tApp('datasource.list.subtitle') }}</p>
      </div>
    </div>

    <div class="list-controls">
      <div class="list-controls-left">
        <input
          id="datasource-search"
          v-model="store.listSearch"
          :placeholder="tApp('datasource.list.searchPlaceholder')"
          autocapitalize="off"
          autocorrect="off"
          spellcheck="false"
        />
        <div class="select-group">
          <span class="select-label">{{ tApp('datasource.list.sortLabel') }}</span>
          <select id="datasource-sort" v-model="store.listSort">
            <option value="name-asc">{{ tApp('datasource.list.sort.nameAsc') }}</option>
            <option value="name-desc">{{ tApp('datasource.list.sort.nameDesc') }}</option>
            <option value="type-asc">{{ tApp('datasource.list.sort.typeAsc') }}</option>
            <option value="status">{{ tApp('datasource.list.sort.status') }}</option>
          </select>
        </div>
      </div>
      <div class="list-controls-right">
        <button class="btn secondary" type="button" @click="testAll">{{ tApp('datasource.list.testAll') }}</button>
        <button class="btn" type="button" @click="openCreate">{{ tApp('datasource.list.new') }}</button>
      </div>
    </div>

    <div class="cards" id="datasource-list">
      <div v-if="filtered.length === 0" class="card">
        <div class="meta">{{ tApp('datasource.list.empty') }}</div>
        <div class="actions">
          <button class="btn" type="button" @click="openCreate">{{ tApp('datasource.list.create') }}</button>
        </div>
      </div>

      <div
        v-for="ds in filtered"
        :key="ds.id"
        class="card datasource-card"
        :class="statusClass(ds.id)"
      >
        <div class="card-header">
          <h3>{{ ds.name }}</h3>
        </div>
        <div class="card-body">
          <div class="meta">
            <span class="datasource-type" :class="datasourceTypeClass(ds.type)">
              <img
                v-if="getDatasourceTypeIconUrl(ds.type)"
                class="datasource-type-icon"
                :src="getDatasourceTypeIconUrl(ds.type)!"
                :alt="tApp('datasource.list.typeLogoAlt', { type: datasourceTypeLabel(ds.type) })"
                loading="lazy"
              />
              {{ datasourceTypeLabel(ds.type) }}
            </span>
            <span v-if="databaseMetaLabel(ds)">{{ databaseMetaLabel(ds) }}</span>
          </div>
          <div class="endpoint-row">
            <span class="endpoint-text" :title="endpointLabel(ds)">
              {{ endpointLabel(ds) || '-' }}
            </span>
            <button
              class="copy-button endpoint-copy"
              type="button"
              :aria-label="tApp('datasource.list.copyEndpoint')"
              :title="tApp('datasource.list.copyEndpoint')"
              data-testid="datasource-endpoint-copy"
              @click="copyEndpoint(ds)"
            >
              <svg class="copy-icon" viewBox="0 0 24 24" aria-hidden="true">
                <rect
                  class="copy-icon-back"
                  x="4"
                  y="6"
                  width="12"
                  height="12"
                  rx="2"
                />
                <rect
                  class="copy-icon-front"
                  x="8"
                  y="4"
                  width="12"
                  height="12"
                  rx="2"
                />
              </svg>
            </button>
          </div>
        </div>
        <div class="card-footer">
          <div class="status-detail-row" data-testid="datasource-status-detail-row">
            <div class="status-detail-left">
              <div
                v-if="statusDetail(ds.id)"
                class="status"
                :class="[statusBadgeClass(ds.id), { 'is-flash': flashTestId === ds.id }]"
                data-testid="datasource-status-badge"
                :data-datasource-id="ds.id"
              >
                {{ statusLabel(ds.id) }}
              </div>
              <span class="status-detail-text" :title="statusDetail(ds.id)">
                {{ statusDetail(ds.id) || '' }}
              </span>
            </div>
            <span
              v-if="statusDetail(ds.id) && statusCheckedAtLabel(ds.id)"
              class="status-meta status-meta-inline"
            >
              {{ tApp('datasource.list.checkedAt', { time: statusCheckedAtLabel(ds.id) }) }}
            </span>
            <button
              v-if="statusDetail(ds.id)"
              class="copy-button status-copy"
              type="button"
              :aria-label="tApp('datasource.list.copyError')"
              :title="tApp('datasource.list.copyError')"
              data-testid="datasource-status-copy"
              @click="copyStatusDetail(ds)"
            >
              <svg class="copy-icon" viewBox="0 0 24 24" aria-hidden="true">
                <rect
                  class="copy-icon-back"
                  x="4"
                  y="6"
                  width="12"
                  height="12"
                  rx="2"
                />
                <rect
                  class="copy-icon-front"
                  x="8"
                  y="4"
                  width="12"
                  height="12"
                  rx="2"
                />
              </svg>
            </button>
          </div>
          <div v-if="!statusDetail(ds.id)" class="status-row status-row-bottom">
            <div
              class="status"
              :class="[statusBadgeClass(ds.id), { 'is-flash': flashTestId === ds.id }]"
              data-testid="datasource-status-badge"
              :data-datasource-id="ds.id"
            >
              {{ statusLabel(ds.id) }}
            </div>
            <span v-if="statusCheckedAtLabel(ds.id)" class="status-meta">
              {{ tApp('datasource.list.checkedAt', { time: statusCheckedAtLabel(ds.id) }) }}
            </span>
          </div>
          <div class="actions actions-tight">
            <button class="btn" type="button" @click="openConsole(ds)">{{ tApp('datasource.list.openConsole') }}</button>
            <button class="btn secondary small" type="button" @click="testDatasource(ds)">{{ tApp('common.test') }}</button>
            <button
              v-if="shouldShowD1ReAuthentication(ds)"
              :class="['btn secondary small', { 'is-loading': isD1ReAuthenticationLoading(ds.id) }]"
              type="button"
              :disabled="isD1ReAuthenticationLoading(ds.id)"
              data-testid="d1-reauth-button"
              :data-datasource-id="ds.id"
              @click="reAuthenticateD1Datasource(ds)"
            >
              {{
                isD1ReAuthenticationLoading(ds.id)
                  ? tApp('datasource.list.d1ReAuthenticationLoading')
                  : tApp('datasource.list.d1ReAuthentication')
              }}
            </button>
            <button
              v-if="shouldShowDynamoReAuthentication(ds)"
              :class="['btn secondary small', { 'is-loading': isDynamoReAuthenticationLoading(ds.id) }]"
              type="button"
              :disabled="isDynamoReAuthenticationLoading(ds.id)"
              data-testid="dynamodb-reauth-button"
              :data-datasource-id="ds.id"
              @click="reAuthenticateDynamoDatasource(ds)"
            >
              {{
                isDynamoReAuthenticationLoading(ds.id)
                  ? tApp('datasource.list.dynamoReAuthenticationLoading')
                  : tApp('datasource.list.dynamoReAuthentication')
              }}
            </button>
            <button class="btn ghost small" type="button" @click="editDatasource(ds)">{{ tApp('common.edit') }}</button>
            <button class="btn ghost danger small delete-btn" type="button" @click="openDelete(ds)">
              {{ tApp('common.delete') }}
            </button>
          </div>
        </div>
      </div>
    </div>

    <div
      v-if="deleteTarget"
      class="dialog-backdrop"
      role="dialog"
      aria-modal="true"
      data-testid="datasource-delete-dialog"
    >
      <div class="dialog-card dialog-card--danger">
        <div class="dialog-head">
          <div class="dialog-head-main">
            <div class="dialog-icon danger"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg></div>
            <div>
              <h4>{{ tApp('datasource.list.deleteTitle') }}</h4>
              <div class="meta">
                <span>{{ tApp('common.cannotUndo') }}</span>
              </div>
            </div>
          </div>
          <span class="pill pill-danger">{{ tApp('common.delete') }}</span>
        </div>
        <div class="dialog-highlight">{{ deleteTarget.name }}</div>
        <div class="meta">
          <span>{{ datasourceTypeLabel(deleteTarget.type) }} · {{ deleteTarget.host }}{{ deleteTarget.port ? `:${deleteTarget.port}` : '' }}</span>
        </div>
        <div class="dialog-actions">
          <button class="btn ghost" type="button" :disabled="deleteBusy" @click="closeDelete">{{ tApp('common.cancel') }}</button>
          <button
            class="btn danger"
            type="button"
            :disabled="deleteBusy"
            data-testid="datasource-delete-confirm"
            @click="confirmDelete"
          >
            {{ tApp('common.delete') }}
          </button>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { useDatasourceListView } from './datasource-list/useDatasourceListView'
import { getDatasourceTypeIconUrl } from '@/modules/datasource/icons'
import { tApp } from '@/modules/i18n/appI18n'

const {
  store,
  filtered,
  datasourceTypeLabel,
  datasourceTypeClass,
  databaseMetaLabel,
  endpointLabel,
  copyEndpoint,
  statusLabel,
  statusBadgeClass,
  statusClass,
  statusDetail,
  flashTestId,
  statusCheckedAtLabel,
  shouldShowD1ReAuthentication,
  isD1ReAuthenticationLoading,
  reAuthenticateD1Datasource,
  shouldShowDynamoReAuthentication,
  isDynamoReAuthenticationLoading,
  reAuthenticateDynamoDatasource,
  copyStatusDetail,
  openCreate,
  editDatasource,
  openConsole,
  testDatasource,
  testAll,
  deleteTarget,
  deleteBusy,
  openDelete,
  closeDelete,
  confirmDelete,
} = useDatasourceListView()
</script>
