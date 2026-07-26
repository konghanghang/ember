import { request } from './request'
import type {
  RedeemCodeRequest,
  RedeemCodeResponse,
  RedemptionCode,
  RedemptionListResponse,
  TmdbSearchResponse,
  TmdbTVSeasonOptionsResponse
} from '@/types/api'

export function redeemCode(data: RedeemCodeRequest): Promise<RedeemCodeResponse> {
	return request({
		url: '/redeem',
		method: 'post',
		data
	})
}

export function validateRedeemCode(code: string): Promise<RedemptionCode> {
	return request({
		url: `/redeem/${code}/validate`,
		method: 'get'
	})
}

export function getRedemptions(params?: { page?: number; pageSize?: number }): Promise<RedemptionListResponse> {
	return request({
		url: '/redemptions',
		method: 'get',
		params
	})
}

// TMDB Search
export function searchTmdb(query: string, type: 'movie' | 'tv', opts?: { silent?: boolean }): Promise<TmdbSearchResponse> {
  return request({
    url: '/tmdb/search',
    method: 'get',
    params: { query, type },
    silent: opts?.silent === true
  })
}

export function getTmdbTVSeasons(id: number | string): Promise<TmdbTVSeasonOptionsResponse> {
  return request({
    url: `/tmdb/tv/${id}/seasons`,
    method: 'get'
  })
}
