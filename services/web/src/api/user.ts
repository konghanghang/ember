import request from './request'
import type {
  RedeemCodeRequest,
  RedeemCodeResponse,
  RedemptionCode,
  RedemptionListResponse,
  TmdbSearchResponse
} from '@/types/api'

export function redeemCode(data: RedeemCodeRequest): Promise<RedeemCodeResponse> {
	return request({
		url: '/user/redeem',
		method: 'post',
		data
	})
}

export function validateRedeemCode(code: string): Promise<RedemptionCode> {
	return request({
		url: `/user/redeem/${code}/validate`,
		method: 'get'
	})
}

export function getRedemptions(params?: { page?: number; pageSize?: number }): Promise<RedemptionListResponse> {
	return request({
		url: '/user/redemptions',
		method: 'get',
		params
	})
}

// TMDB Search
export function searchTmdb(query: string, type: 'movie' | 'tv'): Promise<TmdbSearchResponse> {
  return request({
    url: '/tmdb/search',
    method: 'get',
    params: { query, type }
  })
}
