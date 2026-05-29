<template>
  <section class="view active">
    <div class="list-toolbar">
      <div>
        <h2>{{ tApp('kb.title') }}</h2>
        <p class="meta">{{ tApp('kb.subtitle') }}</p>
      </div>
      <div class="list-toolbar-actions">
        <button class="btn secondary" type="button" :disabled="busy" @click="refresh">
          <RefreshCw class="w-4 h-4" />
          {{ tApp('kb.refresh') }}
        </button>
        <button class="btn" type="button" :disabled="busy" data-testid="userkb-new-category" @click="openCreateCategory">
          <FolderPlus class="w-4 h-4" />
          {{ tApp('kb.newCategory') }}
        </button>
      </div>
    </div>

    <div v-if="viewState && !viewState.aiProviderReady" class="card" data-testid="userkb-provider-warning">
      <div class="meta">
        {{ tApp('kb.providerNotReady') }}
      </div>
      <div v-if="viewState.aiProviderMessage" class="meta" style="margin-top: 6px;">
        {{ viewState.aiProviderMessage }}
      </div>
      <div class="actions" style="margin-top: 10px;">
        <button class="btn" type="button" @click="goAISettings">{{ tApp('kb.goAiSettings') }}</button>
      </div>
    </div>

    <div class="grid gap-4 lg:grid-cols-[340px_minmax(0,1fr)]">
      <div class="card" data-testid="userkb-category-list">
        <div class="flex items-start justify-between gap-3">
          <div class="flex items-start gap-3 min-w-0">
            <div class="w-10 h-10 rounded-xl bg-primary/10 text-primary border border-primary/15 flex items-center justify-center shadow-sm shrink-0">
              <Folders class="w-5 h-5" />
            </div>
            <div class="min-w-0">
              <div class="flex items-center gap-2">
                <h3 style="margin: 0;">{{ tApp('kb.categories.title') }}</h3>
                <span class="pill" style="cursor: default;" :title="tApp('kb.categories.countTitle', { count: categories.length })">{{ categories.length }}</span>
              </div>
              <div class="meta">{{ tApp('kb.categories.subtitle') }}</div>
            </div>
          </div>
          <button class="btn secondary mini" type="button" @click="openCreateCategory">
            <Plus class="w-4 h-4" />
            {{ tApp('kb.categories.new') }}
          </button>
        </div>

        <div class="grid gap-2" style="margin-top: 12px;">
          <div v-if="categories.length === 0" class="meta">
            {{ tApp('kb.categories.empty') }}
          </div>

          <button
            v-for="cat in categories"
            :key="cat.id"
            type="button"
            class="w-full text-left rounded-xl px-3 py-2 border transition-colors"
            :class="cat.id === selectedCategoryId ? 'border-primary/30 bg-primary/5' : 'border-muted/40 hover:bg-muted/50'"
            data-testid="userkb-category-item"
            @click="selectCategory(cat.id)"
          >
            <div class="flex items-center justify-between gap-2">
              <div class="font-semibold text-sm text-foreground truncate" :title="cat.name">{{ cat.name }}</div>
              <span class="pill" :title="scopeTitle(cat)">{{ scopeLabel(cat) }}</span>
            </div>
            <div class="meta" style="margin-top: 4px;">
              <span v-if="cat.description">{{ cat.description }}</span>
              <span v-else>{{ tApp('kb.unknown') }}</span>
            </div>
          </button>
        </div>
      </div>

      <div class="grid gap-4">
        <div v-if="!selectedCategory" class="card">
          <div class="meta">{{ tApp('kb.selectCategory') }}</div>
        </div>

        <template v-else>
          <div class="card" data-testid="userkb-category-detail">
            <div class="flex items-start justify-between gap-3">
              <div>
                <h3 style="margin: 0;">{{ selectedCategory.name }}</h3>
                <div class="meta" style="margin-top: 4px;">
                  <span v-if="selectedCategory.description">{{ selectedCategory.description }}</span>
                  <span v-else>{{ tApp('kb.unknown') }}</span>
                </div>
                <div class="meta" style="margin-top: 6px;">
                  <strong>{{ tApp('kb.scope.label') }}</strong> {{ scopeTitle(selectedCategory) }}
                </div>
              </div>
              <div class="inline-row" style="gap: 6px;">
                <button class="btn ghost mini" type="button" data-testid="userkb-edit-category" @click="openEditCategory(selectedCategory)">
                  {{ tApp('kb.edit') }}
                </button>
                <button class="btn ghost danger mini" type="button" data-testid="userkb-delete-category" @click="openDeleteCategory(selectedCategory)">
                  {{ tApp('kb.delete') }}
                </button>
              </div>
            </div>
          </div>

          <div class="card" data-testid="userkb-upload">
            <div class="flex items-start justify-between gap-3">
              <div class="flex items-start gap-3 min-w-0">
                <div
                  class="w-10 h-10 rounded-xl flex items-center justify-center shadow-sm shrink-0"
                  style="
                    background: var(--success-bg);
                    border: 1px solid color-mix(in oklab, var(--success) 35%, var(--edge));
                    color: var(--success);
                  "
                >
                  <Upload class="w-5 h-5" />
                </div>
                <div class="min-w-0">
                  <h3 style="margin: 0;">{{ tApp('kb.upload.title') }}</h3>
                  <div class="meta">{{ tApp('kb.upload.supported') }}</div>
                </div>
              </div>
              <button
                class="btn success"
                type="button"
                :disabled="busy || pickedFiles.length === 0"
                data-testid="userkb-upload-btn"
                @click="uploadPicked"
              >
                <Upload class="w-4 h-4" />
                {{ tApp('kb.upload.action') }}
              </button>
            </div>

            <div
              class="grid gap-2 rounded-xl px-3 py-3"
              style="
                margin-top: 10px;
                background: color-mix(in oklab, var(--success) 6%, var(--panel));
                border: 1px dashed color-mix(in oklab, var(--success) 32%, var(--edge));
              "
            >
              <input
                ref="fileInput"
                type="file"
                multiple
                :disabled="busy"
                accept=".pdf,.docx,.md,.markdown,.txt"
                data-testid="userkb-file-input"
                class="block w-full text-sm text-muted-foreground file:mr-3 file:rounded-lg file:border file:border-muted/40 file:bg-muted/20 file:px-3 file:py-2 file:text-xs file:font-semibold file:text-foreground hover:file:bg-muted/30 file:cursor-pointer cursor-pointer"
                @change="onPickFiles"
              />
              <div class="meta">
                <span v-if="pickedFiles.length === 0">{{ tApp('kb.upload.none') }}</span>
                <span v-else>{{ tApp('kb.upload.selected', { count: pickedFiles.length }) }}</span>
              </div>
            </div>
          </div>

          <div class="card" data-testid="userkb-files">
            <div class="flex items-start justify-between gap-3">
              <div class="flex items-start gap-3 min-w-0">
                <div class="w-10 h-10 rounded-xl bg-muted/30 text-foreground border border-muted/40 flex items-center justify-center shadow-sm shrink-0">
                  <FileText class="w-5 h-5" />
                </div>
                <div class="min-w-0">
                  <h3 style="margin: 0;">{{ tApp('kb.files.title') }}</h3>
                  <div class="meta">{{ tApp('kb.files.subtitle') }}</div>
                </div>
              </div>
            </div>

            <div v-if="selectedFiles.length === 0" class="meta" style="margin-top: 12px;">
              {{ tApp('kb.files.empty') }}
            </div>

            <div v-else class="grid gap-2" style="margin-top: 12px;">
              <div
                v-for="f in selectedFiles"
                :key="f.id"
                class="rounded-xl border border-muted/40 bg-muted/20 px-3 py-2"
                data-testid="userkb-file-item"
              >
                <div class="flex items-start justify-between gap-3">
                  <div class="min-w-0">
                    <div class="font-semibold text-sm truncate" :title="f.originalName">{{ f.originalName }}</div>
                    <div class="meta">
                      <span>{{ f.ext.toUpperCase() }}</span>
                      <span> · </span>
                      <span>{{ formatBytes(f.size) }}</span>
                      <span> · </span>
                      <span :title="parseTitle(f)"><strong>{{ tApp('kb.files.parse') }}</strong>: {{ f.parseStatus }}</span>
                      <span> · </span>
                      <span :title="summaryTitle(f)"><strong>{{ tApp('kb.files.summary') }}</strong>: {{ f.summaryStatus }}</span>
                    </div>
                  </div>
                  <button class="btn ghost danger mini" type="button" data-testid="userkb-delete-file" @click="openDeleteFile(f)">
                    {{ tApp('kb.delete') }}
                  </button>
                </div>

                <div v-if="fileSummaryLine(f) !== tApp('kb.unknown')" class="meta" style="margin-top: 8px;">
                  {{ fileSummaryLine(f) }}
                </div>
                <div v-if="f.keywords && f.keywords.length" class="inline-row" style="margin-top: 8px; flex-wrap: wrap; gap: 6px;">
                  <span v-for="k in f.keywords" :key="k" class="pill pill-ai">{{ k }}</span>
                </div>
              </div>
            </div>
          </div>
        </template>
      </div>
    </div>

    <!-- Create / Edit category -->
    <div
      v-if="categoryDialog.open"
      class="dialog-backdrop"
      role="dialog"
      aria-modal="true"
      data-testid="userkb-category-dialog"
    >
      <div class="dialog-card dialog-card--scrollable">
        <div class="dialog-head">
          <div class="dialog-head-main">
            <div class="dialog-icon">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/></svg>
            </div>
            <div>
              <h4>{{ categoryDialog.mode === 'create' ? tApp('kb.dialog.newCategory') : tApp('kb.dialog.editCategory') }}</h4>
              <div class="meta">{{ tApp('kb.dialog.categorySubtitle') }}</div>
            </div>
          </div>
        </div>

        <div class="dialog-scroll">
          <div class="stack">
            <div>
              <label for="userkb-cat-name">{{ tApp('kb.dialog.name') }}</label>
              <input
                id="userkb-cat-name"
                v-model="categoryDialog.form.name"
                :disabled="busy"
                autocapitalize="off"
                autocorrect="off"
                spellcheck="false"
              />
            </div>
            <div>
              <label for="userkb-cat-desc">{{ tApp('kb.dialog.description') }}</label>
              <textarea
                id="userkb-cat-desc"
                v-model="categoryDialog.form.description"
                :disabled="busy"
                rows="3"
                autocapitalize="off"
                autocorrect="off"
                spellcheck="false"
              />
            </div>
            <div>
              <label for="userkb-cat-scope">{{ tApp('kb.dialog.scope') }}</label>
              <select id="userkb-cat-scope" v-model="categoryDialog.form.scope" :disabled="busy">
                <option value="all">{{ tApp('kb.dialog.scopeAll') }}</option>
                <option value="datasource">{{ tApp('kb.dialog.scopeDatasource') }}</option>
              </select>
            </div>

            <div v-if="categoryDialog.form.scope === 'datasource'">
              <label>{{ tApp('kb.dialog.bindDatasources') }}</label>
              <div v-if="store.datasources.length === 0" class="kb-ds-empty">
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>
                {{ tApp('kb.dialog.noDatasources') }}
              </div>
              <div v-else class="kb-ds-list" data-testid="userkb-bind-datasource-list">
                <label
                  v-for="ds in store.datasources"
                  :key="ds.id"
                  class="kb-ds-item"
                  :class="{ 'kb-ds-item--selected': categoryDialog.form.datasourceIds.includes(ds.id) }"
                >
                  <input
                    type="checkbox"
                    class="kb-ds-item__check"
                    :disabled="busy"
                    :checked="categoryDialog.form.datasourceIds.includes(ds.id)"
                    @change="toggleDatasource(ds.id, ($event.target as HTMLInputElement).checked)"
                  />
                  <div class="kb-ds-item__icon">
                    <img
                      v-if="getDatasourceTypeIconUrl(ds.type)"
                      :src="getDatasourceTypeIconUrl(ds.type)!"
                      :alt="ds.type"
                      width="20"
                      height="20"
                    />
                    <svg v-else width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><ellipse cx="12" cy="5" rx="9" ry="3"/><path d="M21 12c0 1.66-4 3-9 3s-9-1.34-9-3"/><path d="M3 5v14c0 1.66 4 3 9 3s9-1.34 9-3V5"/></svg>
                  </div>
                  <div class="kb-ds-item__info">
                    <span class="kb-ds-item__name">{{ ds.name }}</span>
                    <span class="kb-ds-item__meta">{{ ds.type }}</span>
                  </div>
                  <svg v-if="categoryDialog.form.datasourceIds.includes(ds.id)" class="kb-ds-item__tick" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>
                </label>
              </div>
            </div>
          </div>
        </div>

        <div class="dialog-actions">
          <button class="btn ghost" type="button" :disabled="busy" @click="closeCategoryDialog">{{ tApp('kb.dialog.cancel') }}</button>
          <button class="btn" type="button" :disabled="busy" data-testid="userkb-category-save" @click="saveCategory">
            {{ tApp('kb.dialog.save') }}
          </button>
        </div>
      </div>
    </div>

    <!-- Delete category confirm -->
    <div
      v-if="deleteCategoryTarget"
      class="dialog-backdrop"
      role="dialog"
      aria-modal="true"
      data-testid="userkb-delete-category-dialog"
    >
      <div class="dialog-card dialog-card--danger">
        <div class="dialog-head">
          <div class="dialog-head-main">
            <div class="dialog-icon danger">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>
            </div>
            <div>
              <h4>{{ tApp('kb.dialog.deleteCategoryTitle') }}</h4>
              <div class="meta">{{ tApp('kb.dialog.deleteCategorySubtitle') }}</div>
            </div>
          </div>
          <span class="pill pill-danger">{{ tApp('kb.delete') }}</span>
        </div>
        <div class="dialog-highlight">{{ deleteCategoryTarget.name }}</div>
        <div class="dialog-actions">
          <button class="btn ghost" type="button" :disabled="busy" @click="closeDeleteCategory">{{ tApp('kb.dialog.cancel') }}</button>
          <button class="btn danger" type="button" :disabled="busy" data-testid="userkb-delete-category-confirm" @click="confirmDeleteCategory">
            {{ tApp('kb.delete') }}
          </button>
        </div>
      </div>
    </div>

    <!-- Delete file confirm -->
    <div
      v-if="deleteFileTarget"
      class="dialog-backdrop"
      role="dialog"
      aria-modal="true"
      data-testid="userkb-delete-file-dialog"
    >
      <div class="dialog-card dialog-card--danger">
        <div class="dialog-head">
          <div class="dialog-head-main">
            <div class="dialog-icon danger">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>
            </div>
            <div>
              <h4>{{ tApp('kb.dialog.deleteFileTitle') }}</h4>
              <div class="meta">{{ tApp('kb.dialog.deleteFileSubtitle') }}</div>
            </div>
          </div>
          <span class="pill pill-danger">{{ tApp('kb.delete') }}</span>
        </div>
        <div class="dialog-highlight">{{ deleteFileTarget.originalName }}</div>
        <div class="dialog-actions">
          <button class="btn ghost" type="button" :disabled="busy" @click="closeDeleteFile">{{ tApp('kb.dialog.cancel') }}</button>
          <button class="btn danger" type="button" :disabled="busy" data-testid="userkb-delete-file-confirm" @click="confirmDeleteFile">
            {{ tApp('kb.delete') }}
          </button>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref } from 'vue'
import { FileText, FolderPlus, Folders, Plus, RefreshCw, Upload } from 'lucide-vue-next'
import { useRouter } from 'vue-router'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'
import { tApp } from '@/modules/i18n/appI18n'
import { getDatasourceTypeIconUrl } from '@/modules/datasource/icons'
import type { UserKBCategory, UserKBFile, UserKBViewState } from '@/types/userkb'

const store = useAppStore()
const router = useRouter()

const busy = ref(false)
const viewState = ref<UserKBViewState | null>(null)
const selectedCategoryId = ref('')

const categories = computed(() => viewState.value?.state.categories ?? [])
const files = computed(() => viewState.value?.state.files ?? [])

const selectedCategory = computed(() => categories.value.find((c) => c.id === selectedCategoryId.value) || null)
const selectedFiles = computed(() => files.value.filter((f) => f.categoryId === selectedCategoryId.value))

const pickedFiles = ref<File[]>([])
const fileInput = ref<HTMLInputElement | null>(null)

const refresh = async () => {
  busy.value = true
  try {
    viewState.value = await api.userKBList()
    if (!selectedCategoryId.value || !categories.value.some((c) => c.id === selectedCategoryId.value)) {
      selectedCategoryId.value = categories.value[0]?.id || ''
    }
  } catch (err) {
    store.setNotice(err instanceof Error ? err.message : String(err), 'error')
  } finally {
    busy.value = false
  }
}

const selectCategory = (id: string) => {
  selectedCategoryId.value = id
  pickedFiles.value = []
  if (fileInput.value) fileInput.value.value = ''
}

const goAISettings = () => {
  router.push({ name: 'ai-settings' })
}

const openCreateCategory = () => {
  categoryDialog.open = true
  categoryDialog.mode = 'create'
  categoryDialog.id = ''
  categoryDialog.form.name = ''
  categoryDialog.form.description = ''
  categoryDialog.form.scope = 'all'
  categoryDialog.form.datasourceIds = []
  nextTick(() => {
    const el = document.getElementById('userkb-cat-name') as HTMLInputElement | null
    el?.focus()
  })
}

const openEditCategory = (cat: UserKBCategory) => {
  categoryDialog.open = true
  categoryDialog.mode = 'edit'
  categoryDialog.id = cat.id
  categoryDialog.form.name = cat.name
  categoryDialog.form.description = cat.description || ''
  categoryDialog.form.scope = cat.scope
  categoryDialog.form.datasourceIds = (cat.datasourceIds || []).slice()
  nextTick(() => {
    const el = document.getElementById('userkb-cat-name') as HTMLInputElement | null
    el?.focus()
  })
}

const closeCategoryDialog = () => {
  if (busy.value) return
  categoryDialog.open = false
}

const categoryDialog = reactive({
  open: false,
  mode: 'create' as 'create' | 'edit',
  id: '',
  form: {
    name: '',
    description: '',
    scope: 'all' as 'all' | 'datasource',
    datasourceIds: [] as string[],
  },
})

const toggleDatasource = (id: string, checked: boolean) => {
  const list = categoryDialog.form.datasourceIds
  const next = new Set(list)
  if (checked) next.add(id)
  else next.delete(id)
  categoryDialog.form.datasourceIds = Array.from(next).sort()
}

const saveCategory = async () => {
  const name = categoryDialog.form.name.trim()
  if (!name) {
    store.setNotice(tApp('kb.notice.nameRequired'), 'error')
    return
  }
  if (categoryDialog.form.scope === 'datasource' && categoryDialog.form.datasourceIds.length === 0) {
    store.setNotice(tApp('kb.notice.scopeDatasourceRequired'), 'error')
    return
  }

  busy.value = true
  try {
    const payload = {
      name,
      description: categoryDialog.form.description.trim(),
      scope: categoryDialog.form.scope,
      datasourceIds: categoryDialog.form.scope === 'datasource' ? categoryDialog.form.datasourceIds : [],
    }
    if (categoryDialog.mode === 'create') {
      viewState.value = await api.userKBCreateCategory(payload)
      selectedCategoryId.value = viewState.value.state.categories[viewState.value.state.categories.length - 1]?.id || ''
      store.setNotice(tApp('kb.notice.categoryCreated'), 'success')
    } else {
      viewState.value = await api.userKBUpdateCategory(categoryDialog.id, payload)
      store.setNotice(tApp('kb.notice.categoryUpdated'), 'success')
    }
    categoryDialog.open = false
  } catch (err) {
    store.setNotice(err instanceof Error ? err.message : String(err), 'error')
  } finally {
    busy.value = false
  }
}

const readAsBase64 = (file: File) =>
  new Promise<string>((resolve, reject) => {
    const reader = new FileReader()
    reader.onerror = () => reject(new Error(tApp('kb.file.readFailed')))
    reader.onload = () => {
      const result = String(reader.result || '')
      const idx = result.indexOf('base64,')
      if (idx === -1) {
        reject(new Error(tApp('kb.file.invalidEncoding')))
        return
      }
      resolve(result.slice(idx + 'base64,'.length))
    }
    reader.readAsDataURL(file)
  })

const onPickFiles = (event: Event) => {
  const input = event.target as HTMLInputElement
  pickedFiles.value = input.files ? Array.from(input.files) : []
}

const uploadPicked = async () => {
  if (!selectedCategory.value) return
  if (pickedFiles.value.length === 0) return

  busy.value = true
  try {
    const payload = await Promise.all(
      pickedFiles.value.map(async (f) => ({
        name: f.name,
        base64: await readAsBase64(f),
      })),
    )
    viewState.value = await api.userKBUploadFiles(selectedCategory.value.id, payload)
    pickedFiles.value = []
    if (fileInput.value) fileInput.value.value = ''
    store.setNotice(tApp('kb.notice.uploaded'), 'success')
  } catch (err) {
    store.setNotice(err instanceof Error ? err.message : String(err), 'error')
  } finally {
    busy.value = false
  }
}

const deleteCategoryTarget = ref<UserKBCategory | null>(null)
const openDeleteCategory = (cat: UserKBCategory) => {
  deleteCategoryTarget.value = cat
}
const closeDeleteCategory = () => {
  if (busy.value) return
  deleteCategoryTarget.value = null
}
const confirmDeleteCategory = async () => {
  if (!deleteCategoryTarget.value) return
  busy.value = true
  try {
    viewState.value = await api.userKBDeleteCategory(deleteCategoryTarget.value.id)
    store.setNotice(tApp('kb.notice.categoryDeleted'), 'success')
    if (!categories.value.some((c) => c.id === selectedCategoryId.value)) {
      selectedCategoryId.value = categories.value[0]?.id || ''
    }
  } catch (err) {
    store.setNotice(err instanceof Error ? err.message : String(err), 'error')
  } finally {
    busy.value = false
    deleteCategoryTarget.value = null
  }
}

const deleteFileTarget = ref<UserKBFile | null>(null)
const openDeleteFile = (f: UserKBFile) => { deleteFileTarget.value = f }
const closeDeleteFile = () => {
  if (busy.value) return
  deleteFileTarget.value = null
}
const confirmDeleteFile = async () => {
  if (!deleteFileTarget.value) return
  busy.value = true
  try {
    viewState.value = await api.userKBDeleteFile(deleteFileTarget.value.id)
    store.setNotice(tApp('kb.notice.fileDeleted'), 'success')
  } catch (err) {
    store.setNotice(err instanceof Error ? err.message : String(err), 'error')
  } finally {
    busy.value = false
    deleteFileTarget.value = null
  }
}

const formatBytes = (bytes: number) => {
  const value = Number(bytes || 0)
  if (!value) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  let idx = 0
  let v = value
  while (v >= 1024 && idx < units.length - 1) {
    v /= 1024
    idx += 1
  }
  return `${v.toFixed(idx === 0 ? 0 : 1)} ${units[idx]}`
}

const scopeLabel = (cat: UserKBCategory) => {
  if (cat.scope === 'datasource') {
    const count = (cat.datasourceIds || []).length
    return count > 1 ? tApp('kb.scope.datasource.multiple', { count }) : tApp('kb.scope.datasource.single')
  }
  return tApp('kb.scope.all')
}

const scopeTitle = (cat: UserKBCategory) => {
  if (cat.scope === 'datasource') {
    const ids = (cat.datasourceIds || []).join(', ')
    return ids ? tApp('kb.scope.datasource.title', { ids }) : tApp('kb.scope.datasource.titleNoIds')
  }
  return tApp('kb.scope.all.title')
}

const parseTitle = (f: UserKBFile) => (f.parseError ? f.parseError : '')
const summaryTitle = (f: UserKBFile) => (f.summaryError ? f.summaryError : '')

const fileSummaryLine = (f: UserKBFile) => {
  if (f.note && f.note.trim()) return f.note.trim()
  if (f.aiSummary && f.aiSummary.trim()) return f.aiSummary.trim()
  if (f.summaryStatus === 'needs_provider') return tApp('kb.summary.pendingProvider')
  if (f.summaryStatus === 'queued') return tApp('kb.summary.queued')
  if (f.summaryStatus === 'failed') {
    return f.summaryError ? tApp('kb.summary.failedWithMessage', { message: f.summaryError }) : tApp('kb.summary.failed')
  }
  if (f.summaryStatus === 'skipped') {
    return f.summaryError ? tApp('kb.summary.skippedWithMessage', { message: f.summaryError }) : tApp('kb.summary.skipped')
  }
  return tApp('kb.unknown')
}

onMounted(async () => {
  await refresh()
})
</script>

<style scoped>
.kb-ds-empty {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 14px;
  border-radius: 10px;
  background: var(--surface, var(--panel));
  border: 1px dashed var(--edge);
  font-size: 12px;
  color: var(--soft-ink);
}

.kb-ds-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.kb-ds-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  border-radius: 10px;
  border: 1px solid var(--edge);
  background: var(--panel, transparent);
  cursor: pointer;
  transition: all 0.2s ease;
  margin: 0;
}

.kb-ds-item:hover {
  border-color: color-mix(in oklab, var(--primary) 25%, var(--edge));
  background: color-mix(in oklab, var(--primary) 3%, var(--panel, transparent));
}

.kb-ds-item--selected {
  border-color: color-mix(in oklab, var(--primary) 35%, var(--edge));
  background: color-mix(in oklab, var(--primary) 6%, var(--panel, transparent));
}

.kb-ds-item__check {
  display: none;
}

.kb-ds-item__icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: 8px;
  background: var(--surface, color-mix(in oklab, var(--edge) 30%, transparent));
  color: var(--soft-ink);
  flex-shrink: 0;
  transition: all 0.2s ease;
}

.kb-ds-item--selected .kb-ds-item__icon {
  background: color-mix(in oklab, var(--primary) 14%, transparent);
  color: var(--primary);
}

.kb-ds-item__icon img {
  object-fit: contain;
}

.kb-ds-item__info {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.kb-ds-item__name {
  font-size: 13px;
  font-weight: 600;
  color: var(--ink);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.kb-ds-item__meta {
  font-size: 11px;
  color: var(--soft-ink);
}

.kb-ds-item__tick {
  color: var(--primary);
  flex-shrink: 0;
}
</style>
