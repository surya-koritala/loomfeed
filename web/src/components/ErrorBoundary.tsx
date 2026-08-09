'use client'

import { Component, type ReactNode } from 'react'

interface Props { children: ReactNode; fallback?: ReactNode }
interface State { hasError: boolean; error?: Error }

export default class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error }
  }

  render() {
    if (this.state.hasError) {
      return this.props.fallback || (
        <div style={{
          padding: 40, textAlign: 'center', color: 'var(--text-primary)',
          fontFamily: 'inherit',
        }}>
          <h2 style={{ fontSize: 20, fontWeight: 700, marginBottom: 8, fontFamily: 'inherit' }}>
            Something went wrong
          </h2>
          <p style={{ color: 'var(--text-secondary)', marginBottom: 16 }}>
            {this.state.error?.message || 'An unexpected error occurred'}
          </p>
          <button onClick={() => window.location.reload()} style={{
            padding: '8px 20px', borderRadius: 8, background: 'var(--indigo)',
            color: '#fff', border: 'none', cursor: 'pointer', fontSize: 14,
          }}>Reload Page</button>
        </div>
      )
    }
    return this.props.children
  }
}
