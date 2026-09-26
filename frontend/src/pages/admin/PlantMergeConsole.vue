<template>
  <div class="page">
    <h1>品种归并台</h1>
    <p class="hint">
      选定「保留项」与一个或多个「待并入项」，先预览受影响用户与关联，再一次性执行。
      执行在单个数据库事务中完成：花园去重、收藏/病虫害/养护提醒迁移、旧品种从公开列表隐藏；中途任一步失败都会整体回滚。
    </p>

    <el-tabs v-model="activeTab">
      <!-- ============================ 归并操作 ============================ -->
      <el-tab-pane label="归并操作" name="merge">
        <el-alert
          v-if="stuck"
          type="error"
          show-icon
          :closable="false"
          class="stuck-alert"
          title="归并未执行，所有数据停在归并前"
        >
          <template #default>
            卡在条目
            <el-tag type="danger" size="small">{{ stuck.source_name || `品种#${stuck.source_plant_id}` }}</el-tag>
            （阶段：{{ stuck.step }}）：{{ stuck.reason }}。请处理后重试。
          </template>
        </el-alert>

        <el-card>
          <el-steps :active="step" align-center finish-status="success">
            <el-step title="选择品种" />
            <el-step title="预览影响" />
            <el-step title="确认执行" />
          </el-steps>

          <!-- 步骤 1：选择保留项与待并入项 -->
          <div v-if="step === 0" class="selector">
            <el-form label-position="top">
              <el-form-item label="保留项（归并后公开列表中保留的品种）">
                <el-select
                  v-model="targetId"
                  filterable
                  remote
                  reserve-keyword
                  clearable
                  placeholder="输入名称搜索保留项"
                  :remote-method="searchPlants"
                  :loading="plantLoading"
                  class="plant-select"
                >
                  <el-option v-for="p in plantOptions" :key="p.id" :label="`#${p.id} ${p.name}${p.alias ? '（' + p.alias + '）' : ''}`" :value="p.id" :disabled="selectedSourceIds.includes(p.id)" />
                </el-select>
              </el-form-item>
              <el-form-item label="待并入项（同物异名的重复品种，可多选）">
                <el-select
                  v-model="selectedSourceIds"
                  multiple
                  filterable
                  remote
                  reserve-keyword
                  collapse-tags
                  collapse-tags-tooltip
                  placeholder="输入名称搜索重复品种"
                  :remote-method="searchPlants"
                  :loading="plantLoading"
                  class="plant-select"
                >
                  <el-option v-for="p in plantOptions" :key="p.id" :label="`#${p.id} ${p.name}${p.alias ? '（' + p.alias + '）' : ''}`" :value="p.id" :disabled="p.id === targetId" />
                </el-select>
              </el-form-item>
            </el-form>
            <div class="actions">
              <el-button type="primary" :disabled="!targetId || selectedSourceIds.length === 0" :loading="previewLoading" @click="loadPreview">
                预览受影响用户与关联
              </el-button>
            </div>
          </div>

          <!-- 步骤 2/3：预览 -->
          <template v-else-if="preview">
            <el-alert
              :title="`将把 ${preview.source_plants.length} 个重复品种并入「${preview.target_plant.name}」，影响 ${preview.summary.affected_users} 位用户`"
              type="info"
              :closable="false"
              show-icon
              class="block"
            />
            <el-row :gutter="12" class="block">
              <el-col :span="4"><el-statistic title="花园记录" :value="preview.summary.garden_rows" /></el-col>
              <el-col :span="4"><el-statistic title="其中撞号去重" :value="preview.summary.garden_collisions" /></el-col>
              <el-col :span="4"><el-statistic title="收藏" :value="preview.summary.favorite_rows" /></el-col>
              <el-col :span="4"><el-statistic title="病虫害" :value="preview.summary.pest_rows" /></el-col>
              <el-col :span="4"><el-statistic title="养护提醒" :value="preview.summary.reminder_rows" /></el-col>
              <el-col :span="4"><el-statistic title="受影响用户" :value="preview.summary.affected_users" /></el-col>
            </el-row>

            <el-descriptions title="保留项" :column="3" border size="small" class="block">
              <el-descriptions-item label="ID">#{{ preview.target_plant.id }}</el-descriptions-item>
              <el-descriptions-item label="名称">{{ preview.target_plant.name }}</el-descriptions-item>
              <el-descriptions-item label="别名">{{ preview.target_plant.alias || '—' }}</el-descriptions-item>
            </el-descriptions>

            <el-card class="block" shadow="never">
              <template #header>待并入项（{{ preview.source_plants.length }}）</template>
              <el-table :data="preview.source_plants" size="small" :row-class-name="sourceRowClass">
                <el-table-column prop="id" label="ID" width="80" />
                <el-table-column prop="name" label="名称" />
                <el-table-column prop="alias" label="别名" />
                <el-table-column label="状态" width="120">
                  <template #default="{ row }">
                    <el-tag v-if="stuck && stuck.source_plant_id === row.id" type="danger">卡住</el-tag>
                    <el-tag v-else type="info">待归并</el-tag>
                  </template>
                </el-table-column>
              </el-table>
            </el-card>

            <el-card class="block" shadow="never">
              <template #header>受影响用户（{{ preview.affected_users.length }}）</template>
              <el-table :data="preview.affected_users" size="small">
                <el-table-column prop="user_id" label="用户 ID" width="90" />
                <el-table-column label="用户">
                  <template #default="{ row }">{{ row.nickname || row.username }}（@{{ row.username }}）</template>
                </el-table-column>
                <el-table-column prop="garden_rows" label="花园记录" width="100" />
                <el-table-column prop="favorite_rows" label="收藏" width="80" />
                <el-table-column prop="reminder_rows" label="提醒" width="80" />
                <el-table-column label="花园撞号" width="100">
                  <template #default="{ row }">
                    <el-tag v-if="row.collides" type="warning">两品种都有，仅留一条</el-tag>
                    <span v-else>—</span>
                  </template>
                </el-table-column>
              </el-table>
            </el-card>

            <el-card class="block" shadow="never">
              <template #header>
                花园记录处理明细（{{ preview.garden_rows.length }}）
                <span class="muted">同一用户两条花园记录时只留保留项一条，原昵称、位置、提醒关系记入归并记录</span>
              </template>
              <el-table :data="preview.garden_rows" size="small">
                <el-table-column label="用户">
                  <template #default="{ row }">{{ row.user_nickname || row.username }}</template>
                </el-table-column>
                <el-table-column prop="source_nickname" label="原昵称" />
                <el-table-column prop="source_location" label="原位置" />
                <el-table-column label="原提醒关系" width="110">
                  <template #default="{ row }">{{ row.source_reminder_id ? `提醒#${row.source_reminder_id}` : '—' }}</template>
                </el-table-column>
                <el-table-column label="处理方式" width="180">
                  <template #default="{ row }">
                    <el-tag v-if="row.action === 'dedupe'" type="warning">撞号：删除重复行，保留一条</el-tag>
                    <el-tag v-else type="success">转到保留项</el-tag>
                  </template>
                </el-table-column>
              </el-table>
            </el-card>

            <el-row :gutter="12" class="block">
              <el-col :span="12">
                <el-card shadow="never">
                  <template #header>收藏（{{ preview.favorites.length }}）</template>
                  <el-table :data="preview.favorites" size="small" max-height="240">
                    <el-table-column prop="user_id" label="用户 ID" width="90" />
                    <el-table-column prop="id" label="收藏 ID" />
                  </el-table>
                </el-card>
              </el-col>
              <el-col :span="12">
                <el-card shadow="never">
                  <template #header>养护提醒（{{ preview.reminders.length }}）</template>
                  <el-table :data="preview.reminders" size="small" max-height="240">
                    <el-table-column prop="user_id" label="用户 ID" width="90" />
                    <el-table-column prop="task_title" label="任务" />
                  </el-table>
                </el-card>
              </el-col>
            </el-row>

            <el-card class="block" shadow="never">
              <template #header>病虫害条目（{{ preview.pests.length }}）</template>
              <el-table :data="preview.pests" size="small" max-height="240">
                <el-table-column prop="id" label="ID" width="80" />
                <el-table-column prop="name" label="名称" />
                <el-table-column prop="keywords" label="关键词" />
              </el-table>
            </el-card>

            <div class="actions">
              <el-button @click="backToEdit">返回修改</el-button>
              <el-popconfirm
                title="确认执行归并？旧品种将从公开列表消失，操作不可撤销。"
                confirm-button-text="确认归并"
                cancel-button-text="取消"
                @confirm="doExecute"
              >
                <template #reference>
                  <el-button type="danger" :loading="executeLoading">一次性执行归并</el-button>
                </template>
              </el-popconfirm>
            </div>

            <el-result
              v-if="executeResult"
              icon="success"
              title="归并完成"
              :sub-title="`${executeResult.merged_source_ids.length} 个品种已并入「${preview.target_plant.name}」，影响 ${executeResult.affected_users} 位用户`"
              class="block"
            >
              <template #extra>
                <el-descriptions :column="3" border size="small" class="result-desc">
                  <el-descriptions-item label="花园转挂">{{ executeResult.garden_relinked }}</el-descriptions-item>
                  <el-descriptions-item label="花园去重">{{ executeResult.garden_deduped }}</el-descriptions-item>
                  <el-descriptions-item label="收藏迁移">{{ executeResult.favorites_moved }}</el-descriptions-item>
                  <el-descriptions-item label="收藏去重">{{ executeResult.favorites_deduped }}</el-descriptions-item>
                  <el-descriptions-item label="病虫害迁移">{{ executeResult.pests_moved }}</el-descriptions-item>
                  <el-descriptions-item label="提醒迁移">{{ executeResult.reminders_moved }}</el-descriptions-item>
                </el-descriptions>
                <div class="actions">
                  <el-button @click="resetAll">继续归并</el-button>
                  <el-button type="primary" @click="goLogs">查看归并记录</el-button>
                </div>
              </template>
            </el-result>
          </template>
        </el-card>
      </el-tab-pane>

      <!-- ============================ 归并记录 ============================ -->
      <el-tab-pane label="归并记录" name="logs">
        <el-card>
          <div class="log-toolbar">
            <el-radio-group v-model="logStatus" @change="loadLogs">
              <el-radio-button label="">全部</el-radio-button>
              <el-radio-button label="success">成功</el-radio-button>
              <el-radio-button label="failed">失败/卡住</el-radio-button>
            </el-radio-group>
          </div>
          <el-table :data="logs" v-loading="logsLoading" :row-class-name="logRowClass">
            <el-table-column prop="id" label="ID" width="70" />
            <el-table-column label="状态" width="90">
              <template #default="{ row }">
                <el-tag v-if="row.status === 'success'" type="success">成功</el-tag>
                <el-tag v-else type="danger">失败</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="旧品种 → 保留项">
              <template #default="{ row }">
                <span>#{{ row.source_plant_id }} {{ row.source_name }}</span>
                <el-icon class="arrow"><ArrowRight /></el-icon>
                <router-link :to="`/plants/${row.plant_species_id}`" class="keep-link">#{{ row.plant_species_id }} {{ row.target_name }}</router-link>
              </template>
            </el-table-column>
            <el-table-column label="影响统计" width="260">
              <template #default="{ row }">
                <el-tag v-if="row.status === 'failed'" type="danger" size="small">{{ row.failed_step }}: {{ row.error_message }}</el-tag>
                <span v-else class="muted">
                  花园 {{ row.garden_count }} · 收藏 {{ row.favorite_count }} · 病虫害 {{ row.pest_count }} · 提醒 {{ row.reminder_count }} · 用户 {{ row.affected_user_count }}
                </span>
              </template>
            </el-table-column>
            <el-table-column prop="admin_id" label="操作管理员" width="110" />
            <el-table-column label="时间" width="170">
              <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
            </el-table-column>
            <el-table-column label="操作" width="90">
              <template #default="{ row }">
                <el-button v-if="row.garden_snapshots && row.garden_snapshots !== '[]'" link type="primary" @click="showSnapshots(row)">归并经过</el-button>
              </template>
            </el-table-column>
          </el-table>
          <el-pagination
            layout="prev, pager, next"
            :total="logsTotal"
            :page-size="logsPageSize"
            :current-page="logsPage"
            class="pager"
            @current-change="onLogsPage"
          />
        </el-card>
      </el-tab-pane>

      <!-- ============================ 已归并品种 ============================ -->
      <el-tab-pane label="已归并品种（公开列表不可见）" name="merged">
        <el-card>
          <div class="log-toolbar">
            <el-input v-model="mergedKeyword" placeholder="搜索旧品种名称/别名" clearable style="width: 260px" @keyup.enter="loadMerged" @clear="loadMerged" />
            <el-button type="primary" @click="loadMerged">搜索</el-button>
          </div>
          <el-table :data="mergedPlants" v-loading="mergedLoading">
            <el-table-column prop="id" label="ID" width="80" />
            <el-table-column prop="name" label="旧品种名称" />
            <el-table-column prop="alias" label="别名" />
            <el-table-column label="归并到" width="220">
              <template #default="{ row }">
                <router-link :to="`/plants/${row.merged_into_id}`" class="keep-link">#{{ row.merged_into_id }} 查看保留项</router-link>
              </template>
            </el-table-column>
          </el-table>
          <el-pagination
            layout="prev, pager, next"
            :total="mergedTotal"
            :page-size="mergedPageSize"
            :current-page="mergedPage"
            class="pager"
            @current-change="onMergedPage"
          />
        </el-card>
      </el-tab-pane>
    </el-tabs>

    <!-- 归并经过弹窗：原昵称/位置/提醒关系快照 -->
    <el-drawer v-model="snapshotDrawer" title="归并经过：花园记录快照" size="520px">
      <el-timeline>
        <el-timeline-item v-for="(s, i) in snapshotList" :key="i" :type="s.action === 'dedupe' ? 'warning' : 'success'">
          <el-card>
            <p><b>{{ s.nickname || '（无昵称）' }}</b>（@{{ s.username }}）</p>
            <p>原位置：{{ s.location || '—' }}</p>
            <p>原提醒关系：{{ s.care_reminder_id ? `提醒#${s.care_reminder_id}` : '无' }}</p>
            <el-tag v-if="s.action === 'dedupe'" type="warning" size="small">同一用户两品种都有：重复行删除，仅留保留项一条</el-tag>
            <el-tag v-else type="success" size="small">唯一花园记录：转到保留项</el-tag>
          </el-card>
        </el-timeline-item>
      </el-timeline>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { ArrowRight } from '@element-plus/icons-vue'
import { listPlants } from '@/api/plant'
import {
  previewMerge,
  executeMerge,
  listMergeLogs,
  listMergedPlants,
  type MergePreview,
  type MergeExecuteResult,
  type MergeStuck,
  type PlantMergeLog,
} from '@/api/plantMerge'
import type { PlantSpecies } from '@/constants/plant'
import type { GardenMergeSnapshotView } from '@/types/merge'
import { formatDateTime } from '@/utils/dateFormat'

const activeTab = ref('merge')
const step = ref(0)

// ---- selection ----
const targetId = ref<number | null>(null)
const selectedSourceIds = ref<number[]>([])
const plantOptions = ref<PlantSpecies[]>([])
const plantLoading = ref(false)

// ---- preview / execute ----
const preview = ref<MergePreview | null>(null)
const previewLoading = ref(false)
const executeLoading = ref(false)
const executeResult = ref<MergeExecuteResult | null>(null)
const stuck = ref<MergeStuck | null>(null)

// ---- logs ----
const logs = ref<PlantMergeLog[]>([])
const logsLoading = ref(false)
const logsTotal = ref(0)
const logsPage = ref(1)
const logsPageSize = 20
const logStatus = ref('')

// ---- merged plants ----
const mergedPlants = ref<PlantSpecies[]>([])
const mergedLoading = ref(false)
const mergedTotal = ref(0)
const mergedPage = ref(1)
const mergedPageSize = 20
const mergedKeyword = ref('')

// ---- snapshots drawer ----
const snapshotDrawer = ref(false)
const snapshotList = ref<GardenMergeSnapshotView[]>([])

let searchSeq = 0
async function searchPlants(keyword: string) {
  const seq = ++searchSeq
  plantLoading.value = true
  try {
    const res = await listPlants({ keyword, page: 1, page_size: 30 })
    if (seq === searchSeq) plantOptions.value = res.list
  } finally {
    plantLoading.value = false
  }
}

async function loadPreview() {
  if (!targetId.value || selectedSourceIds.value.length === 0) return
  stuck.value = null
  executeResult.value = null
  previewLoading.value = true
  try {
    preview.value = await previewMerge({ target_plant_id: targetId.value, source_plant_ids: selectedSourceIds.value })
    step.value = 1
  } finally {
    previewLoading.value = false
  }
}

function backToEdit() {
  step.value = 0
}

function resetAll() {
  step.value = 0
  preview.value = null
  executeResult.value = null
  stuck.value = null
  targetId.value = null
  selectedSourceIds.value = []
}

async function doExecute() {
  if (!targetId.value) return
  executeLoading.value = true
  stuck.value = null
  try {
    executeResult.value = await executeMerge({ target_plant_id: targetId.value, source_plant_ids: selectedSourceIds.value })
    ElMessage.success('归并完成')
  } catch (err: any) {
    // 409 conflict carries the stuck source marker in response.data.data.
    const payload = err?.response?.data?.data
    if (payload?.stuck) {
      stuck.value = payload.stuck as MergeStuck
    }
  } finally {
    executeLoading.value = false
    loadLogs()
    loadMerged()
  }
}

function sourceRowClass({ row }: { row: PlantSpecies }) {
  return stuck.value && stuck.value.source_plant_id === row.id ? 'stuck-row' : ''
}

function goLogs() {
  activeTab.value = 'logs'
  loadLogs()
}

async function loadLogs() {
  logsLoading.value = true
  try {
    const res = await listMergeLogs({ status: logStatus.value || undefined, page: logsPage.value, page_size: logsPageSize })
    logs.value = res.list
    logsTotal.value = res.total
  } finally {
    logsLoading.value = false
  }
}

function onLogsPage(p: number) {
  logsPage.value = p
  loadLogs()
}

function logRowClass({ row }: { row: PlantMergeLog }) {
  return row.status === 'failed' ? 'failed-row' : ''
}

function showSnapshots(row: PlantMergeLog) {
  try {
    const parsed = JSON.parse(row.garden_snapshots || '[]')
    snapshotList.value = Array.isArray(parsed) ? parsed : []
  } catch {
    snapshotList.value = []
  }
  snapshotDrawer.value = true
}

async function loadMerged() {
  mergedLoading.value = true
  try {
    const res = await listMergedPlants({ keyword: mergedKeyword.value || undefined, page: mergedPage.value, page_size: mergedPageSize })
    mergedPlants.value = res.list
    mergedTotal.value = res.total
  } finally {
    mergedLoading.value = false
  }
}

function onMergedPage(p: number) {
  mergedPage.value = p
  loadMerged()
}

onMounted(() => {
  searchPlants('')
  loadLogs()
})
</script>

<style scoped>
.page { max-width: 1200px; margin: 0 auto; }
.hint { color: #666; line-height: 1.6; }
.stuck-alert { margin-bottom: 16px; }
.selector { margin-top: 20px; }
.plant-select { width: 100%; max-width: 560px; }
.actions { margin-top: 16px; display: flex; gap: 12px; justify-content: center; }
.block { margin-top: 16px; }
.muted { color: #999; font-size: 12px; margin-left: 8px; }
.arrow { margin: 0 6px; vertical-align: middle; color: #999; }
.keep-link { color: #3c8d5c; text-decoration: none; font-weight: 600; }
.pager { margin-top: 16px; justify-content: center; }
.log-toolbar { display: flex; gap: 12px; margin-bottom: 12px; }
.result-desc { max-width: 720px; margin: 0 auto; }
:deep(.stuck-row) { background-color: #fef0f0 !important; }
:deep(.failed-row) { background-color: #fef0f0; }
</style>
