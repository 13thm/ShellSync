// Package auth handles authentication tokens.
//
// Responsibilities (design §3.1):
//   - issue/verify the local token used by the Desktop client (lock file)
//   - issue/verify session tokens for paired Mobile devices
//   - bcrypt-hashed storage of session tokens
//
// Implemented in M2-3.
package auth
