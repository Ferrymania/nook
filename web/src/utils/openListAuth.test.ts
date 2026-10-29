import { openListAuthConfig, openListAuthError, type OpenListAuthFields } from './openListAuth'

function check(name: string, condition: boolean) {
  if (!condition) throw new Error(`openListAuth: ${name}`)
}

const fields: OpenListAuthFields = {
  server: ' https://list.example.com ', token: ' token ', hasToken: true,
}
const tokenConfig = openListAuthConfig(fields)
check('only address and Token are submitted', Object.keys(tokenConfig).sort().join(',') === 'server,token')
check('Token and address are trimmed', tokenConfig.token === 'token' && tokenConfig.server === 'https://list.example.com')
const savedToken = { ...fields, token: '' }
check('omitting saved Token keeps it unchanged', !('token' in openListAuthConfig(savedToken)))
check('saved Token is valid', !openListAuthError(savedToken))
check('missing Token does not fall back to password', !!openListAuthError({ ...savedToken, hasToken: false }))
check('whitespace is not a new Token', !!openListAuthError({ ...savedToken, token: '  ', hasToken: false }))

check('server address is required', !!openListAuthError({ ...fields, server: ' ' }))
check('bare hostname is rejected', !!openListAuthError({ ...fields, server: 'list.example.com' }))
check('unsupported protocol is rejected', !!openListAuthError({ ...fields, server: 'ftp://list.example.com' }))
check('incomplete HTTPS prefix is rejected', !!openListAuthError({ ...fields, server: 'https:list.example.com' }))
check('invalid port is rejected', !!openListAuthError({ ...fields, server: 'http://localhost:bad' }))
check('HTTP with port is supported', !openListAuthError({ ...fields, server: 'http://192.168.1.10:5244' }))
check('HTTPS domain is supported', !openListAuthError({ ...fields, server: 'https://list.example.com' }))

console.log('openListAuth tests passed')
