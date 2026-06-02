import type { MediaLibraryOption, UserMediaLibraryItem } from '@/types/api'

type MediaLibrarySummarySource = Pick<MediaLibraryOption | UserMediaLibraryItem, 'type' | 'itemCount'>

/** 格式化媒体库副信息；只有后端明确返回条目数时才展示数量，避免把未知误写成 0。 */
export function formatMediaLibrarySummary(library: MediaLibrarySummarySource): string {
  const type = library.type || 'Unknown'
  return typeof library.itemCount === 'number' ? `${type} · ${library.itemCount} 项` : type
}
