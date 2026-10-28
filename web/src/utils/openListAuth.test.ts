import { defaultOpenListAuthMode, openListAuthConfig, openListAuthError, type OpenListAuthFields } from './openListAuth'

function check(name: string, condition: boolean) {
  if (!condition) throw new Error(`openListAuth: ${name}`)
}

check('new accounts default to Token', defaultOpenListAuthMode() === 'token')
check('empty accounts default to Token', defaultOpenListAuthMode({}) === 'token')
check('saved passwords retain their mode', defaultOpenListAuthMode({ has_password: true }) === 'password')
check('Token takes precedence over saved password', defaultOpenListAuthMode({ has_password: true, has_token: true }) === 'token')

const fields: OpenListAuthFields = {
  mode: 'token', server: ' https://list.example.com ', username: ' alice ',
  password: ' password with spaces ', token: ' token ', hasPassword: true, hasToken: true,
}
const tokenConfig = openListAuthConfig(fields)
check('Token mode clears inactive credentials', tokenConfig.username === '' && tokenConfig.password === '')
check('Token and address are trimmed', tokenConfig.token === 'token' && tokenConfig.server === 'https://list.example.com')
const savedToken = { ...fields, token: '' }
check('omitting saved Token keeps it unchanged', !('token' in openListAuthConfig(savedToken)))
check('saved Token is valid', !openListAuthError(savedToken))
check('missing Token does not fall back to password', !!openListAuthError({ ...savedToken, hasToken: false }))
check('whitespace is not a new Token', !!openListAuthError({ ...savedToken, token: '  ', hasToken: false }))

const passwordFields: OpenListAuthFields = { ...fields, mode: 'password' }
const passwordConfig = openListAuthConfig(passwordFields)
check('password mode clears Token', passwordConfig.token === '')
check('username trimmed, password preserved', passwordConfig.username === 'alice' && passwordConfig.password === fields.password)
const savedPassword = { ...passwordFields, password: '' }
check('omitting saved password keeps it unchanged', !('password' in openListAuthConfig(savedPassword)))
check('saved password is valid', !openListAuthError(savedPassword))
check('password mode requires username', !!openListAuthError({ ...passwordFields, username: ' ' }))
check('missing password does not fall back to Token', !!openListAuthError({ ...savedPassword, hasPassword: false }))
check('all modes require server address', !!openListAuthError({ ...fields, server: ' ' }))
check('bare hostname is rejected', !!openListAuthError({ ...fields, server: 'list.example.com' }))
check('unsupported protocol is rejected', !!openListAuthError({ ...fields, server: 'ftp://list.example.com' }))
check('incomplete HTTPS prefix is rejected', !!openListAuthError({ ...fields, server: 'https:list.example.com' }))
check('invalid port is rejected', !!openListAuthError({ ...fields, server: 'http://localhost:bad' }))
check('HTTP with port is supported', !openListAuthError({ ...fields, server: 'http://192.168.1.10:5244' }))
check('HTTPS domain is supported', !openListAuthError({ ...fields, server: 'https://list.example.com' }))

console.log('openListAuth tests passed')
