// Package pty provides a cross-platform pseudo-terminal abstraction.
//
// Responsibilities (design §3.1):
//   - spawn native shells (cmd/powershell/bash/zsh)
//   - expose a unified Read/Write/Resize/Close interface
//   - Windows uses ConPTY, Unix uses creack/pty
//
// Implemented in M1-6.
package pty
