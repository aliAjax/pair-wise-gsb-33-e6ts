<template>
  <div class="page">
    <h1>品种归并台</h1>
    <p class="hint">
      同物异名的品种在此归并：选择「保留项」与「待并入项」，先预览受影响用户和花园、收藏、病虫害、养护提醒关联，
      再一次性执行。同一用户同时持有两条花园记录时只保留保留项那条，原昵称、位置与提醒关系记入归并记录。
    </p>

    <el-card>
      <template #header>第一步 · 选择品种</template>
      <el-form label-position="top" class="merge-form">
        <el-form-item label="保留项（归并后公开可见的品种）">
          <el-select
            v-model="keepId"
            filterable
            remote
            clearable
            reserve-keyword
            placeholder="按名称搜索保留项"
            :remote-method="searchKeep"
            :loading="keepLoading"
            class="plant-select"
            @change="onSelectionChange"
          >
            <el-option
              v-for="p in keepOptions"
              :key="p.id"
              :label="`${p.name}（#${p.id}）`"
              :value="p.id"
              :disabled="p.id === sourceId"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="待并入项（归并后从公开列表消失的品种）">
          <el-select
            v-model="sourceId"
            filterable
            remote
            clearable
            reserve-keyword
            placeholder="按名称搜索待并入项"
            :remote-method="searchSource"
            :loading="sourceLoading"
            class="plant-select"
            @change="onSelectionChange"
          >
            <el-option
              v-for="p in sourceOptions"
              :key="p.id"
              :label="`${p.name}（#${p.id}）`"
              :value="p.id"
              :disabled="p.id === keepId"
            />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :disabled="!keepId || !sourceId" :loading="previewLoading" @click="doPreview">
            预览受影响用户与关联
          </el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card v-if="preview" class="block">
      <template #header>
        第二步 · 归并预览
        <span class="preview-arrow">{{ preview.source_plant_name }}（#{{ preview.source_plant_id }}）→ {{ preview.keep_plant_name }}（#{{ preview.keep_plant_id }}）</span>
      </template>
      <el-alert type="info" :closable="false" class="block" show-icon>
        共影响 <b>{{ preview.affected_users.length }}</b> 位用户：
        花园 {{ preview.gardens.length }} 条、收藏 {{ preview.favorites.length }} 条、
        病虫害 {{ preview.pests.length }} 条、养护提醒 {{ preview.reminders.length }} 条。
        其中花园去重删除 {{ gardenDedupeCount }} 条、收藏去重删除 {{ favoriteDedupeCount }} 条。
      </el-alert>

      <el-tabs class="block">
        <el-tab-pane :label="`受影响用户（${preview.affected_users.length}）`">
          <el-table :data="preview.affected_users" empty-text="无关联用户">
            <el-table-column prop="user_id" label="用户 ID" width="100" />
            <el-table-column prop="username" label="用户名" />
            <el-table-column prop="nickname" label="昵称" />
          </el-table>
        </el-tab-pane>
        <el-tab-pane :label="`花园记录（${preview.gardens.length}）`">
          <el-table :data="preview.gardens" empty-text="无花园记录" :row-class-name="gardenRowClass">
            <el-table-column prop="user_id" label="用户 ID" width="90" />
            <el-table-column prop="username" label="用户名" width="120" />
            <el-table-column prop="nickname" label="原昵称" />
            <el-table-column prop="location" label="位置" />
            <el-table-column label="提醒关系" width="120">
              <template #default="{ row }">
                <span v-if="row.care_reminder_id">提醒 #{{ row.care_reminder_id }}</span>
                <span v-else class="muted">未关联</span>
              </template>
            </el-table-column>
            <el-table-column label="处理方式" width="130">
              <template #default="{ row }">
                <el-tag v-if="row.action === 'move'" type="primary">转到保留项</el-tag>
                <el-tag v-else type="warning">重复·删除存档</el-tag>
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>
        <el-tab-pane :label="`收藏（${preview.favorites.length}）`">
          <el-table :data="preview.favorites" empty-text="无收藏关联">
            <el-table-column prop="user_id" label="用户 ID" width="100" />
            <el-table-column prop="username" label="用户名" />
            <el-table-column label="处理方式" width="150">
              <template #default="{ row }">
                <el-tag v-if="row.action === 'move'" type="primary">转到保留项</el-tag>
                <el-tag v-else type="warning">重复·去重删除</el-tag>
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>
        <el-tab-pane :label="`病虫害（${preview.pests.length}）`">
          <el-table :data="preview.pests" empty-text="无病虫害关联">
            <el-table-column prop="pest_id" label="条目 ID" width="120" />
            <el-table-column prop="name" label="病虫害名称" />
          </el-table>
        </el-tab-pane>
        <el-tab-pane :label="`养护提醒（${preview.reminders.length}）`">
          <el-table :data="preview.reminders" empty-text="无提醒关联">
            <el-table-column prop="reminder_id" label="提醒 ID" width="100" />
            <el-table-column prop="username" label="用户名" width="140" />
            <el-table-column prop="task_title" label="任务" />
            <el-table-column prop="remind_date" label="提醒日期" width="130" />
          </el-table>
        </el-tab-pane>
      </el-tabs>

      <el-alert
        type="warning"
        :closable="false"
        show-icon
        class="block"
        title="执行在单个事务中完成：中途任一步出错，花园、收藏、病虫害和品种记录都会停在归并前；失败尝试会在下方记录中标记为「卡住」。"
      />
      <div class="actions">
        <el-button @click="preview = null">取消</el-button>
        <el-popconfirm
          :width="320"
          placement="top"
          confirm-button-text="确认执行归并"
          cancel-button-text="再想想"
          :title="`确认将「${preview.source_plant_name}」并入「${preview.keep_plant_name}」？该操作不可撤销。`"
          @confirm="doExecute"
        >
          <template #reference>
            <el-button type="danger" :loading="executeLoading">确认无误，一次性执行归并</el-button>
          </template>
        </el-popconfirm>
      </div>
    </el-card>

    <el-card class="block">
      <template #header>
        归并记录
        <el-button size="small" class="refresh-btn" @click="loadRecords">刷新</el-button>
      </template>
      <el-table
        :data="records"
        v-loading="recordsLoading"
        :row-class-name="recordRowClass"
        empty-text="暂无归并记录"
      >
        <el-table-column prop="id" label="ID" width="70" />
        <el-table-column label="保留项" min-width="140">
          <template #default="{ row }">{{ row.keep_plant_name }}（#{{ row.keep_plant_id }}）</template>
        </el-table-column>
        <el-table-column label="待并入项" min-width="140">
          <template #default="{ row }">{{ row.source_plant_name }}（#{{ row.source_plant_id }}）</template>
        </el-table-column>
        <el-table-column prop="operator_id" label="操作人 ID" width="100" />
        <el-table-column label="状态" width="120">
          <template #default="{ row }">
            <el-tag v-if="row.status === 'success'" type="success">已完成</el-tag>
            <el-tag v-else type="danger" effect="dark">卡住</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="时间" width="160">
          <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column label="归并经过 / 错误" width="150">
          <template #default="{ row }">
            <el-button size="small" @click="showDetail(row)">查看</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-pagination
        class="pager"
        layout="prev, pager, next, total"
        :total="recordsTotal"
        :page-size="pageSize"
        :current-page="page"
        @current-change="onPageChange"
      />
    </el-card>

    <el-dialog v-model="detailVisible" title="归并经过" width="760px">
      <template v-if="detailRecord">
        <el-descriptions :column="2" border>
          <el-descriptions-item label="保留项">{{ detailRecord.keep_plant_name }}（#{{ detailRecord.keep_plant_id }}）</el-descriptions-item>
          <el-descriptions-item label="待并入项">{{ detailRecord.source_plant_name }}（#{{ detailRecord.source_plant_id }}）</el-descriptions-item>
          <el-descriptions-item label="状态">
            <el-tag v-if="detailRecord.status === 'success'" type="success">已完成</el-tag>
            <el-tag v-else type="danger" effect="dark">卡住</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="时间">{{ formatDateTime(detailRecord.created_at) }}</el-descriptions-item>
          <el-descriptions-item v-if="detailRecord.error" label="错误原因" :span="2">
            <span class="error-text">{{ detailRecord.error }}</span>
          </el-descriptions-item>
        </el-descriptions>
        <template v-if="detailData">
          <el-tabs class="detail-tabs">
            <el-tab-pane :label="`花园（${detailData.gardens.length}）`">
              <el-table :data="detailData.gardens" size="small" max-height="280">
                <el-table-column prop="user_id" label="用户" width="80" />
                <el-table-column prop="nickname" label="原昵称" />
                <el-table-column prop="location" label="位置" />
                <el-table-column label="提醒" width="90">
                  <template #default="{ row }">{{ row.care_reminder_id || '无' }}</template>
                </el-table-column>
                <el-table-column label="处理" width="110">
                  <template #default="{ row }">{{ row.action === 'move' ? '转到保留项' : '重复·删除存档' }}</template>
                </el-table-column>
              </el-table>
            </el-tab-pane>
            <el-tab-pane :label="`收藏（${detailData.favorites.length}）`">
              <el-table :data="detailData.favorites" size="small" max-height="280">
                <el-table-column prop="user_id" label="用户" width="100" />
                <el-table-column label="处理">
                  <template #default="{ row }">{{ row.action === 'move' ? '转到保留项' : '重复·去重删除' }}</template>
                </el-table-column>
              </el-table>
            </el-tab-pane>
            <el-tab-pane :label="`病虫害（${detailData.pests.length}）`">
              <el-table :data="detailData.pests" size="small" max-height="280">
                <el-table-column prop="pest_id" label="ID" width="90" />
                <el-table-column prop="name" label="名称" />
              </el-table>
            </el-tab-pane>
            <el-tab-pane :label="`提醒（${detailData.reminders.length}）`">
              <el-table :data="detailData.reminders" size="small" max-height="280">
                <el-table-column prop="reminder_id" label="ID" width="80" />
                <el-table-column prop="username" label="用户" />
                <el-table-column prop="task_title" label="任务" />
              </el-table>
            </el-tab-pane>
            <el-tab-pane :label="`受影响用户（${detailData.affected_users.length}）`">
              <el-table :data="detailData.affected_users" size="small" max-height="280">
                <el-table-column prop="user_id" label="ID" width="80" />
                <el-table-column prop="username" label="用户名" />
                <el-table-column prop="nickname" label="昵称" />
              </el-table>
            </el-tab-pane>
          </el-tabs>
        </template>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { listPlants } from '@/api/plant'
import {
  executePlantMerge,
  listPlantMergeRecords,
  previewPlantMerge,
  type PlantMergePreview,
  type PlantMergeRecord,
} from '@/api/plantMerge'
import { formatDateTime } from '@/utils/dateFormat'
import type { PlantSpecies } from '@/constants/plant'

const keepId = ref<number>()
const sourceId = ref<number>()
const keepOptions = ref<PlantSpecies[]>([])
const sourceOptions = ref<PlantSpecies[]>([])
const keepLoading = ref(false)
const sourceLoading = ref(false)
const previewLoading = ref(false)
const executeLoading = ref(false)
const preview = ref<PlantMergePreview | null>(null)

const records = ref<PlantMergeRecord[]>([])
const recordsTotal = ref(0)
const recordsLoading = ref(false)
const page = ref(1)
const pageSize = 10

const detailVisible = ref(false)
const detailRecord = ref<PlantMergeRecord | null>(null)
const detailData = ref<PlantMergePreview | null>(null)

const gardenDedupeCount = computed(() => preview.value?.gardens.filter((g) => g.action === 'remove_duplicate').length ?? 0)
const favoriteDedupeCount = computed(() => preview.value?.favorites.filter((f) => f.action === 'remove_duplicate').length ?? 0)

async function searchPlants(keyword: string): Promise<PlantSpecies[]> {
  return (await listPlants({ keyword, page_size: 20 })).list
}

async function searchKeep(query: string) {
  keepLoading.value = true
  try {
    keepOptions.value = await searchPlants(query)
  } finally {
    keepLoading.value = false
  }
}

async function searchSource(query: string) {
  sourceLoading.value = true
  try {
    sourceOptions.value = await searchPlants(query)
  } finally {
    sourceLoading.value = false
  }
}

function onSelectionChange() {
  // The selection is part of the preview; changing it invalidates the old preview.
  preview.value = null
}

async function doPreview() {
  if (!keepId.value || !sourceId.value) return
  previewLoading.value = true
  try {
    preview.value = await previewPlantMerge({ keep_plant_id: keepId.value, source_plant_id: sourceId.value })
  } finally {
    previewLoading.value = false
  }
}

async function doExecute() {
  if (!keepId.value || !sourceId.value) return
  executeLoading.value = true
  try {
    await executePlantMerge({ keep_plant_id: keepId.value, source_plant_id: sourceId.value })
    ElMessage.success('归并已一次性执行完成，旧品种已从公开列表移除')
    preview.value = null
    keepId.value = undefined
    sourceId.value = undefined
    page.value = 1
    await loadRecords()
  } finally {
    executeLoading.value = false
  }
}

async function loadRecords() {
  recordsLoading.value = true
  try {
    const data = await listPlantMergeRecords({ page: page.value, page_size: pageSize })
    records.value = data.list
    recordsTotal.value = data.total
  } finally {
    recordsLoading.value = false
  }
}

async function onPageChange(p: number) {
  page.value = p
  await loadRecords()
}

function showDetail(row: PlantMergeRecord) {
  detailRecord.value = row
  detailData.value = null
  if (row.detail) {
    try {
      detailData.value = JSON.parse(row.detail) as PlantMergePreview
    } catch {
      detailData.value = null
    }
  }
  detailVisible.value = true
}

function gardenRowClass({ row }: { row: { action: string } }) {
  return row.action === 'remove_duplicate' ? 'dedupe-row' : ''
}

function recordRowClass({ row }: { row: PlantMergeRecord }) {
  return row.status === 'failed' ? 'stuck-row' : ''
}

onMounted(async () => {
  const initial = await searchPlants('')
  keepOptions.value = initial
  sourceOptions.value = initial
  await loadRecords()
})
</script>

<style scoped>
.page { max-width: 1200px; margin: 0 auto; }
.hint { color: #666; line-height: 1.7; }
.block { margin-top: 16px; }
.plant-select { width: 100%; max-width: 480px; }
.preview-arrow { margin-left: 12px; color: #909399; font-size: 13px; font-weight: 400; }
.actions { margin-top: 16px; display: flex; justify-content: flex-end; gap: 12px; }
.refresh-btn { margin-left: 12px; }
.pager { margin-top: 16px; justify-content: flex-end; }
.muted { color: #bbb; }
.error-text { color: #f56c6c; word-break: break-all; }
.detail-tabs { margin-top: 12px; }
:deep(.stuck-row) { background-color: #fef0f0 !important; }
:deep(.dedupe-row) { background-color: #fdf6ec; }
</style>
