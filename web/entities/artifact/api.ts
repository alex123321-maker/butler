import { apiRequest } from '@shared/api/client'

export interface ArtifactItem {
  artifact_id: string
  run_id: string
  session_key: string
  artifact_type: string
  title: string
  summary: string
  content_text: string
  content_json: string
  content_format: string
  source_type: string
  source_ref: string
  created_at: string
  updated_at: string
}

export interface ArtifactListResponse {
  artifacts: ArtifactItem[]
}

export interface ArtifactDetailResponse {
  artifact: ArtifactItem
}

export const fetchArtifacts = (query: Record<string, string | number | undefined> = {}) => {
  return apiRequest<ArtifactListResponse>('/api/v2/artifacts', { query })
}

export const fetchArtifactById = (artifactId: string) => {
  return apiRequest<ArtifactDetailResponse>(`/api/v2/artifacts/${artifactId}`)
}
