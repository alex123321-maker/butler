import { apiRequest } from '@shared/api/client'

export interface InputReceipt {
  input_id: string
  session_key: string
  session_sequence: number
  task_id?: string
  run_id?: string
  status: 'queued' | 'running' | 'waiting_for_user' | 'blocked' | 'completed' | 'failed' | 'expired' | 'cancelled'
  checkpoint: string
  error_code?: string
  error_message?: string
  attempts: number
  max_attempts: number
  expires_at: string
}

export const fetchInputReceipt = (id: string) => apiRequest<InputReceipt>(`/api/v3/inputs/${encodeURIComponent(id)}`)
