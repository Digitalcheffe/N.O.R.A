import { useNavigate } from 'react-router-dom'
import { useEffect, useRef, useState, useCallback } from 'react'
import { Topbar } from '../components/Topbar'
import './NotFound.css'

// ── Konami ────────────────────────────────────────────────────────────────────

const KONAMI = [
  'ArrowUp','ArrowUp','ArrowDown','ArrowDown',
  'ArrowLeft','ArrowRight','ArrowLeft','ArrowRight',
  'b','a',
]

// ── Droid click stages ────────────────────────────────────────────────────────

const STAGES = [
  { emoji: '🤖', bleep: 'bleep bloop...' },
  { emoji: '🤖', bleep: 'hey, stop that.' },
  { emoji: '🤖', bleep: 'i said STOP.' },
  { emoji: '😤', bleep: 'you have been warned.' },
  { emoji: '😡', bleep: '...' },
  { emoji: '🦾', bleep: 'FINE. I am R2-D2. Happy now?!' },
]

// ── Hyperspace canvas ─────────────────────────────────────────────────────────

function HyperspaceCanvas({ onEnd }: { onEnd: () => void }) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const onEndRef  = useRef(onEnd)
  onEndRef.current = onEnd

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const ctx = canvas.getContext('2d')!

    const resize = () => {
      canvas.width  = window.innerWidth
      canvas.height = window.innerHeight
    }
    resize()
    window.addEventListener('resize', resize)

    const DURATION = 3500
    const NUM_STARS = 180

    const stars = Array.from({ length: NUM_STARS }, () => ({
      angle: Math.random() * Math.PI * 2,
      dist:  Math.random() * 8,
      speed: 0.8 + Math.random() * 1.2,
      width: 0.6 + Math.random() * 1.2,
    }))

    const start = Date.now()
    let raf: number

    function frame() {
      if (!canvas) return
      const elapsed  = Date.now() - start
      const progress = Math.min(elapsed / DURATION, 1)

      if (progress >= 1) { onEndRef.current(); return }

      const cx = canvas.width  / 2
      const cy = canvas.height / 2
      const maxDist = Math.hypot(cx, cy) + 50

      // Fade trail — heavy at start for "flash" feel, light after
      const alpha = progress < 0.05 ? 0.85 : 0.18
      ctx.fillStyle = `rgba(0,0,0,${alpha})`
      ctx.fillRect(0, 0, canvas.width, canvas.height)

      const accel = 1 + Math.pow(progress, 1.6) * 60

      for (const s of stars) {
        const oldDist = s.dist
        s.dist += s.speed * accel

        if (s.dist > maxDist) {
          s.dist  = Math.random() * 6
          s.speed = 0.8 + Math.random() * 1.2
          continue
        }

        const ox = cx + Math.cos(s.angle) * oldDist
        const oy = cy + Math.sin(s.angle) * oldDist
        const nx = cx + Math.cos(s.angle) * s.dist
        const ny = cy + Math.sin(s.angle) * s.dist

        // Colour: deep blue → cyan → white as we accelerate
        const t  = Math.min(progress * 2, 1)
        const r  = Math.round(40  + t * 215)
        const g  = Math.round(100 + t * 155)
        const b  = 255
        const op = Math.min(0.3 + progress * 0.7, 1)

        ctx.beginPath()
        ctx.moveTo(ox, oy)
        ctx.lineTo(nx, ny)
        ctx.strokeStyle = `rgba(${r},${g},${b},${op})`
        ctx.lineWidth   = s.width * (1 + progress * 3)
        ctx.stroke()
      }

      // Bright centre glow
      const glow = ctx.createRadialGradient(cx, cy, 0, cx, cy, 120 * progress)
      glow.addColorStop(0,   `rgba(180,220,255,${0.15 * progress})`)
      glow.addColorStop(1,   'rgba(0,0,0,0)')
      ctx.fillStyle = glow
      ctx.fillRect(0, 0, canvas.width, canvas.height)

      raf = requestAnimationFrame(frame)
    }

    raf = requestAnimationFrame(frame)
    return () => { cancelAnimationFrame(raf); window.removeEventListener('resize', resize) }
  }, [])

  return <canvas ref={canvasRef} className="nf-canvas" />
}

// ── Page ──────────────────────────────────────────────────────────────────────

export function NotFound() {
  const navigate   = useNavigate()
  const [hyper,   setHyper]  = useState(false)
  const [clicks,  setClicks] = useState(0)
  const [seq,     setSeq]    = useState<string[]>([])

  const stage    = STAGES[Math.min(clicks, STAGES.length - 1)]
  const revealed = clicks >= STAGES.length - 1

  const activateHyper = useCallback(() => {
    setHyper(true)
  }, [])

  const endHyper = useCallback(() => {
    setHyper(false)
  }, [])

  useEffect(() => {
    function handleKey(e: KeyboardEvent) {
      setSeq(prev => {
        const next = [...prev, e.key].slice(-KONAMI.length)
        if (next.join(',') === KONAMI.join(',')) activateHyper()
        return next
      })
    }
    window.addEventListener('keydown', handleKey)
    return () => window.removeEventListener('keydown', handleKey)
  }, [activateHyper])

  void seq

  return (
    <>
      <Topbar title="404" />
      <div className="content nf-page">

        {hyper && (
          <>
            <HyperspaceCanvas onEnd={endHyper} />
            <div className="nf-hs-overlay">
              <div className="nf-hs-title">JUMPING TO HYPERSPACE</div>
              <div className="nf-hs-sub">May the Force be with you.</div>
            </div>
          </>
        )}

        <div className={`nf-inner${hyper ? ' nf-hidden' : ''}`}>
          <div
            className={`nf-droid${revealed ? ' nf-revealed' : ''}`}
            onClick={() => setClicks(c => Math.min(c + 1, STAGES.length - 1))}
            title="click me..."
          >
            <div className="nf-emoji">{stage.emoji}</div>
            <div className={`nf-bleep${clicks > 0 ? ' nf-bleep-angry' : ''}`}>
              {stage.bleep}
            </div>
          </div>

          <div className="nf-wave-text">
            <span key={revealed ? 'r' : 'n'}>
              {revealed
                ? 'Artoo-Detoo, it is you! It is you!'
                : "These are not the droids you're looking for."}
            </span>
          </div>

          <div className="nf-code">4 0 4</div>

          <div className="nf-sub">
            Move along. <span className="nf-dim">Move along.</span>
          </div>

          <button className="nf-btn" onClick={() => navigate('/')}>
            ← Return to base
          </button>

          <div className="nf-hint">↑↑↓↓←→←→BA</div>
        </div>

      </div>
    </>
  )
}
