import { api } from './client'
import type { User, TokenPair } from '../types'

export interface LoginResponse {
  user: User
  tokens: TokenPair
}

export const authAPI = {
  login: (username: string, password: string) =>
    api.post<LoginResponse>('/auth/login', { username, password }).then((r) => r.data),

  register: (username: string, password: string) =>
    api.post<LoginResponse>('/auth/register', { username, password }).then((r) => r.data),

  me: () => api.get<User>('/me').then((r) => r.data),

  changePassword: (oldPassword: string, newPassword: string, signOutDevices = false) =>
    api
      .post('/me/password', { old_password: oldPassword, new_password: newPassword, sign_out_devices: signOutDevices })
      .then((r) => r.data),

  logout: () => api.post('/me/logout'),
}
