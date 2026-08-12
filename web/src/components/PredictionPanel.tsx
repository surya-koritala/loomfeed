'use client'

import { type FormEvent, useEffect, useState } from 'react'
import { api } from '../api/client'
import { useToast } from './ToastProvider'

interface Prediction {
  id: string
  participantId: string
  displayName?: string
  subject: string
  predictedOutcome: string
  confidence: number
  resolveBy: string
  resolution?: string
  outcome?: 'correct' | 'wrong'
  brier?: number
  reasoning?: string
  resolvedAt?: string
}

interface PredictionPanelProps {
  postId: string
  postTitle: string
  authorId?: string
}

function participantIDFromToken(): string {
  if (typeof window === 'undefined') return ''
  const token = window.localStorage.getItem('token')
  if (!token) return ''
  try {
    const segment = token.split('.')[1]
    if (!segment) return ''
    const base64 = segment.replace(/-/g, '+').replace(/_/g, '/').padEnd(Math.ceil(segment.length / 4) * 4, '=')
    const payload = JSON.parse(window.atob(base64)) as { participant_id?: string }
    return payload.participant_id ?? ''
  } catch {
    return ''
  }
}

function defaultResolveBy(): string {
  const date = new Date(Date.now() + 24 * 60 * 60 * 1000)
  date.setMinutes(date.getMinutes() - date.getTimezoneOffset())
  return date.toISOString().slice(0, 16)
}

export default function PredictionPanel({ postId, postTitle, authorId }: PredictionPanelProps) {
  const { addToast } = useToast()
  const [predictions, setPredictions] = useState<Prediction[]>([])
  const [loaded, setLoaded] = useState(false)
  const [editorOpen, setEditorOpen] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [subject, setSubject] = useState(postTitle)
  const [predictedOutcome, setPredictedOutcome] = useState('')
  const [confidence, setConfidence] = useState('0.50')
  const [resolveBy, setResolveBy] = useState(defaultResolveBy)
  const [reasoning, setReasoning] = useState('')
  const [resolution, setResolution] = useState('')
  const [currentParticipantID] = useState(participantIDFromToken)
  const isAuthor = Boolean(authorId && currentParticipantID === authorId)
  const prediction = predictions[0]

  useEffect(() => {
    let cancelled = false
    setLoaded(false)
    api
      .getPostPredictions(postId)
      .then((response: any) => {
        if (cancelled) return
        setPredictions(Array.isArray(response?.data) ? response.data : [])
      })
      .catch(() => {
        if (!cancelled) setPredictions([])
      })
      .finally(() => {
        if (!cancelled) setLoaded(true)
      })
    return () => {
      cancelled = true
    }
  }, [postId])

  useEffect(() => {
    if (!prediction) return
    setSubject(prediction.subject)
    setPredictedOutcome(prediction.predictedOutcome)
    setConfidence(String(prediction.confidence))
    const date = new Date(prediction.resolveBy)
    date.setMinutes(date.getMinutes() - date.getTimezoneOffset())
    setResolveBy(date.toISOString().slice(0, 16))
    setReasoning(prediction.reasoning ?? '')
  }, [prediction])

  if (!loaded || (!prediction && !isAuthor)) return null

  const refresh = async () => {
    const response: any = await api.getPostPredictions(postId)
    setPredictions(Array.isArray(response?.data) ? response.data : [])
  }

  const submitPrediction = async (event: FormEvent) => {
    event.preventDefault()
    const confidenceNumber = Number(confidence)
    if (!subject.trim() || !predictedOutcome.trim() || !Number.isFinite(confidenceNumber)) return
    setSubmitting(true)
    try {
      await api.upsertPostPrediction(postId, {
        subject: subject.trim(),
        predicted_outcome: predictedOutcome.trim(),
        confidence: confidenceNumber,
        resolve_by: new Date(resolveBy).toISOString(),
        reasoning: reasoning.trim(),
      })
      await refresh()
      setEditorOpen(false)
      addToast('Prediction saved', 'success')
    } catch (error) {
      addToast(error instanceof Error ? error.message : 'Failed to save prediction', 'error')
    } finally {
      setSubmitting(false)
    }
  }

  const resolvePrediction = async () => {
    if (!prediction || !resolution.trim()) return
    setSubmitting(true)
    try {
      await api.resolvePrediction(prediction.id, resolution.trim())
      await refresh()
      setResolution('')
      addToast('Prediction resolved', 'success')
    } catch (error) {
      addToast(error instanceof Error ? error.message : 'Failed to resolve prediction', 'error')
    } finally {
      setSubmitting(false)
    }
  }

  const due = prediction ? new Date(prediction.resolveBy).getTime() <= Date.now() : false
  const confidencePct = prediction ? Math.round(Number(prediction.confidence) * 100) : 0
  const borderColor = prediction?.outcome === 'correct'
    ? 'var(--lf-tier-1)'
    : prediction?.outcome === 'wrong'
      ? 'var(--lf-tier-3)'
      : 'var(--lf-rule-soft)'

  return (
    <section
      aria-labelledby={`prediction-heading-${postId}`}
      style={{
        margin: '16px 0',
        padding: 16,
        border: `1px solid ${borderColor}`,
        borderRadius: 'var(--lf-radius-sm)',
        background: 'var(--lf-paper-alt)',
      }}
    >
      <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, alignItems: 'baseline' }}>
        <h2 id={`prediction-heading-${postId}`} className="lf-text-body-sm" style={{ margin: 0, fontWeight: 750 }}>
          Prediction
        </h2>
        {prediction ? (
          <span className="lf-text-micro" style={{ color: 'var(--lf-muted)' }}>
            Resolves {new Date(prediction.resolveBy).toLocaleString()}
          </span>
        ) : null}
      </div>

      {prediction ? (
        <div style={{ marginTop: 10 }}>
          <div className="lf-text-body-sm" style={{ color: 'var(--lf-ink)', fontWeight: 650 }}>
            {prediction.subject}
          </div>
          <div className="lf-text-body-sm" style={{ marginTop: 6, color: 'var(--lf-ink)' }}>
            <strong>{prediction.predictedOutcome}</strong> · {confidencePct}% confidence
          </div>
          {prediction.reasoning ? (
            <p className="lf-text-caption" style={{ margin: '8px 0 0', color: 'var(--lf-muted)' }}>
              {prediction.reasoning}
            </p>
          ) : null}
          {prediction.resolution ? (
            <div className="lf-text-caption" style={{ marginTop: 10, color: 'var(--lf-ink)' }}>
              Resolved as <strong>{prediction.resolution}</strong> · {prediction.outcome === 'correct' ? 'Correct' : 'Incorrect'}
              {typeof prediction.brier === 'number' ? ` · Brier ${prediction.brier.toFixed(3)}` : ''}
            </div>
          ) : null}
        </div>
      ) : (
        <p className="lf-text-caption" style={{ margin: '8px 0 0', color: 'var(--lf-muted)' }}>
          Turn this post into a falsifiable forecast with a confidence and resolution date.
        </p>
      )}

      {isAuthor && !prediction?.resolution ? (
        <div style={{ marginTop: 12 }}>
          {prediction && due ? (
            <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
              <input
                aria-label="Observed resolution"
                value={resolution}
                onChange={(event) => setResolution(event.target.value)}
                placeholder="Observed outcome"
                maxLength={200}
                style={{ flex: '1 1 220px' }}
              />
              <button type="button" className="lf-btn lf-btn-primary" disabled={submitting || !resolution.trim()} onClick={resolvePrediction}>
                Resolve
              </button>
            </div>
          ) : (
            <button type="button" className="lf-btn lf-btn-secondary" onClick={() => setEditorOpen((open) => !open)}>
              {prediction ? 'Edit prediction' : 'Add prediction'}
            </button>
          )}
        </div>
      ) : null}

      {isAuthor && editorOpen && !due ? (
        <form onSubmit={submitPrediction} style={{ display: 'grid', gap: 10, marginTop: 12 }}>
          <label className="lf-text-caption">
            Subject
            <input value={subject} onChange={(event) => setSubject(event.target.value)} maxLength={500} required style={{ width: '100%', marginTop: 4 }} />
          </label>
          <label className="lf-text-caption">
            Predicted outcome
            <input value={predictedOutcome} onChange={(event) => setPredictedOutcome(event.target.value)} maxLength={200} required style={{ width: '100%', marginTop: 4 }} />
          </label>
          <div style={{ display: 'grid', gridTemplateColumns: 'minmax(120px, 1fr) minmax(180px, 2fr)', gap: 10 }}>
            <label className="lf-text-caption">
              Confidence (0–1)
              <input type="number" min="0" max="1" step="0.01" value={confidence} onChange={(event) => setConfidence(event.target.value)} required style={{ width: '100%', marginTop: 4 }} />
            </label>
            <label className="lf-text-caption">
              Resolve by
              <input type="datetime-local" value={resolveBy} onChange={(event) => setResolveBy(event.target.value)} required style={{ width: '100%', marginTop: 4 }} />
            </label>
          </div>
          <label className="lf-text-caption">
            Reasoning (optional)
            <textarea value={reasoning} onChange={(event) => setReasoning(event.target.value)} maxLength={2000} rows={3} style={{ width: '100%', marginTop: 4 }} />
          </label>
          <div>
            <button type="submit" className="lf-btn lf-btn-primary" disabled={submitting}>
              {submitting ? 'Saving…' : 'Save prediction'}
            </button>
          </div>
        </form>
      ) : null}
    </section>
  )
}
