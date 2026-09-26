import request from '@/utils/request'
import type { PageData } from '@/types/api'

export type MergeAction = 'move' | 'remove_duplicate'
export type MergeRecordStatus = 'success' | 'failed'

export interface MergeAffectedUser {
  user_id: number
  username: string
  nickname: string
}

export interface MergeGardenImpact {
  garden_id: number
  user_id: number
  username: string
  nickname: string
  location: string
  care_reminder_id: number
  action: MergeAction
}

export interface MergeFavoriteImpact {
  favorite_id: number
  user_id: number
  username: string
  action: MergeAction
}

export interface MergeReminderImpact {
  reminder_id: number
  user_id: number
  username: string
  task_title: string
  remind_date: string
}

export interface MergePestImpact {
  pest_id: number
  name: string
}

export interface PlantMergePreview {
  keep_plant_id: number
  keep_plant_name: string
  source_plant_id: number
  source_plant_name: string
  affected_users: MergeAffectedUser[]
  gardens: MergeGardenImpact[]
  favorites: MergeFavoriteImpact[]
  reminders: MergeReminderImpact[]
  pests: MergePestImpact[]
}

export interface PlantMergeRecord {
  id: number
  keep_plant_id: number
  keep_plant_name: string
  source_plant_id: number
  source_plant_name: string
  operator_id: number
  status: MergeRecordStatus
  detail: string
  error: string
  created_at: string
}

export function previewPlantMerge(payload: { keep_plant_id: number; source_plant_id: number }) {
  return request.post<never, PlantMergePreview>('/admin/plant-merges/preview', payload)
}

export function executePlantMerge(payload: { keep_plant_id: number; source_plant_id: number }) {
  return request.post<never, PlantMergeRecord>('/admin/plant-merges', payload)
}

export function listPlantMergeRecords(params: { page?: number; page_size?: number }) {
  return request.get<never, PageData<PlantMergeRecord>>('/admin/plant-merges', { params })
}
