import axios, { type AxiosInstance } from 'axios'

/** Daemon unified response envelope. */
interface Envelope<T> {
  code: number
  data: T
  message: string
}

let instance: AxiosInstance | null = null

/** Configure the shared HTTP client once the daemon connection is known. */
export function configureHttp(baseUrl: string, token: string) {
  instance = axios.create({
    baseURL: baseUrl,
    timeout: 15000,
    headers: { Authorization: `Bearer ${token}` },
  })
  // Unwrap {code,data,message}: on success replace res.data with the payload;
  // on non-zero code reject. Resource modules then read res.data (.then(r => r.data)).
  instance.interceptors.response.use(
    (res) => {
      const env = res.data as Envelope<unknown>
      if (env && typeof env.code === 'number') {
        if (env.code === 0) {
          res.data = env.data
          return res
        }
        const err = new Error(env.message || `code ${env.code}`) as Error & {
          code: number
        }
        err.code = env.code
        return Promise.reject(err)
      }
      return res
    },
    (err) => Promise.reject(err),
  )
}

export function http(): AxiosInstance {
  if (!instance) throw new Error('HTTP client not configured (no daemon connection)')
  return instance
}

/** True once configureHttp has been called. */
export function isConfigured(): boolean {
  return instance !== null
}
