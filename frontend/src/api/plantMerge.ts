import request from '@/utils/request'
import type { PageData, CareReminder, DiseasePest } from '@/types/api'
import type { PlantSpecies } from '@/constants/plant'

export interface MergeGardenRow {
  source_garden_id: number
  target_garden_id: number
  user_id: number
  username: string
  user_nickname: string
  source_nickname: string
  source_location: string
  source_reminder_id: number
  collides: boolean
  action: 'dedupe' | 'relink'
}

export interface MergeFavoriteRow {
  id: number
  user_id: number
  target_type: string
  target_id: number
  created_at: string
}

export interface MergePreviewUser {
  user_id: number
  username: string
  nickname: string
  garden_rows: number
  favorite_rows: number
  reminder_rows: number
  collides: boolean
}

export interface MergePreviewSummary {
  garden_rows: number
  garden_collisions: number
  favorite_rows: number
  pest_rows: number
  reminder_rows: number
  affected_users: number
}

export interface MergePreview {
  target_plant: PlantSpecies
  source_plants: PlantSpecies[]
  garden_rows: MergeGardenRow[]
  favorites: MergeFavoriteRow[]
  pests: DiseasePest[]
  reminders: CareReminder[]
  affected_users: MergePreviewUser[]
  summary: MergePreviewSummary
}

export interface MergeStuck {
  source_plant_id: number
  source_name: string
  step: string
  reason: string
}

export interface MergeExecuteResult {
  target_id: number
  merged_source_ids: number[]
  garden_relinked: number
  garden_deduped: number
  favorites_moved: number
  favorites_deduped: number
  pests_moved: number
  reminders_moved: number
  affected_users: number
}

export interface PlantMergeLog {
  id: number
  plant_species_id: number
  target_name: string
  source_plant_id: number
  source_name: string
  source_alias: string
  admin_id: number
  status: 'success' | 'failed'
  failed_step: string
  error_message: string
  garden_count: number
  favorite_count: number
  pest_count: number
  reminder_count: number
  affected_user_count: number
  garden_snapshots: string
  created_at: string
}

export function previewMerge(payload: { target_plant_id: number; source_plant_ids: number[] }) {
  return request.post<never, MergePreview>('/plants/merges/preview', payload)
}

export function executeMerge(payload: { target_plant_id: number; source_plant_ids: number[] }) {
  return request.post<never, MergeExecuteResult>('/plants/merges', payload)
}

export function listMergeLogs(params: { status?: string; page?: number; page_size?: number }) {
  return request.get<never, PageData<PlantMergeLog>>('/plants/merges', { params })
}

export function listMergedPlants(params: { keyword?: string; page?: number; page_size?: number }) {
  return request.get<never, PageData<PlantSpecies>>('/plants/merges/merged', { params })
}
