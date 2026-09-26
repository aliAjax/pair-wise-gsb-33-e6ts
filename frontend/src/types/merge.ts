// GardenMergeSnapshotView mirrors the backend model.GardenMergeSnapshot JSON.
export interface GardenMergeSnapshotView {
  user_id: number
  username: string
  nickname: string
  location: string
  care_reminder_id: number
  action: 'dedupe' | 'relink'
}
