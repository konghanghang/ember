import request from './request'

// User Profile
export function getUserProfile() {
  return request({
    url: '/user/profile',
    method: 'get'
  })
}

export function updateUserProfile(data: any) {
  return request({
    url: '/user/profile',
    method: 'put',
    data
  })
}

export function updateUserEmail(email: string) {
  return request({
    url: '/user/email',
    method: 'put',
    data: { newEmail: email }
  })
}

export function updateUserPassword(data: any) {
  return request({
    url: '/user/password',
    method: 'put',
    data
  })
}

// Media Info
export function getEmbyConfig() {
  return request({
    url: '/emby/config',
    method: 'get'
  })
}

export function getMediaStats() {
  return request({
    url: '/media/stats',
    method: 'get'
  })
}

// Subscriptions
export function getUserSubscriptions() {
  return request({
    url: '/user/subscriptions',
    method: 'get'
  })
}

export function createSubscription(data: any) {
  return request({
    url: '/user/subscriptions',
    method: 'post',
    data
  })
}

export function deleteSubscription(id: string) {
  return request({
    url: `/user/subscriptions/${id}`,
    method: 'delete'
  })
}

// TMDB Search
export function searchTmdb(query: string, type: 'movie' | 'tv') {
  return request({
    url: '/tmdb/search',
    method: 'get',
    params: { query, type }
  })
}
